package epub

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"

	pdfReader "github.com/ledongthuc/pdf"
)

// Metadata is the subset of book metadata we try to extract from a file before
// the user finalizes an upload. Fields are best-effort; empty strings mean
// "couldn't detect, ask the user."
type Metadata struct {
	Title  string `json:"title"`
	Author string `json:"author"`
}

// ExtractMetadata picks the right extractor based on file extension. Returns
// zero-value Metadata + nil error when nothing can be detected (caller decides
// what to do — typically fall back to filename).
func ExtractMetadata(path, ext string) (Metadata, error) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "epub":
		return ExtractEPUBMetadata(path)
	case "pdf":
		return ExtractPDFMetadata(path)
	case "md", "markdown":
		return ExtractMarkdownMetadata(path)
	}
	return Metadata{}, nil
}

// ----- EPUB -----

type opfMetadata struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title   []string `xml:"title"`
		Creator []string `xml:"creator"`
		Meta    []struct {
			Name    string `xml:"name,attr"`
			Content string `xml:"content,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Guide struct {
		References []struct {
			Type string `xml:"type,attr"`
			Href string `xml:"href,attr"`
		} `xml:"reference"`
	} `xml:"guide"`
}

func ExtractEPUBMetadata(path string) (Metadata, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Metadata{}, err
	}
	defer zr.Close()

	// Find the OPF root file. The container.xml at META-INF/container.xml
	// points to the OPF, but for nearly all EPUBs it lives at OEBPS/content.opf
	// or content.opf. Try the manifest first; fall back to a glob.
	opfPath, _ := findOPFPath(&zr.Reader)
	if opfPath == "" {
		for _, f := range zr.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
				opfPath = f.Name
				break
			}
		}
	}
	if opfPath == "" {
		return Metadata{}, errors.New("no .opf manifest found in EPUB")
	}

	var opfFile *zip.File
	for _, f := range zr.File {
		if f.Name == opfPath {
			opfFile = f
			break
		}
	}
	if opfFile == nil {
		return Metadata{}, errors.New("opf path not present in archive")
	}
	rc, err := opfFile.Open()
	if err != nil {
		return Metadata{}, err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return Metadata{}, err
	}

	var meta opfMetadata
	if err := xml.Unmarshal(body, &meta); err != nil {
		return Metadata{}, err
	}
	out := Metadata{}
	if len(meta.Metadata.Title) > 0 {
		out.Title = strings.TrimSpace(meta.Metadata.Title[0])
	}
	if len(meta.Metadata.Creator) > 0 {
		out.Author = strings.TrimSpace(meta.Metadata.Creator[0])
	}
	return out, nil
}

type opfContainer struct {
	Rootfiles struct {
		Rootfile []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

func findOPFPath(zr *zip.Reader) (string, error) {
	for _, f := range zr.File {
		if f.Name != "META-INF/container.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		var c opfContainer
		if err := xml.Unmarshal(body, &c); err != nil {
			return "", err
		}
		if len(c.Rootfiles.Rootfile) > 0 {
			return c.Rootfiles.Rootfile[0].FullPath, nil
		}
	}
	return "", nil
}

// ExtractEPUBCover returns the cover image bytes + file extension (".jpg",
// ".png", …) for an EPUB at the given path. Tries, in order:
//
//  1. <item properties="cover-image"> in the manifest (EPUB 3 standard).
//  2. <meta name="cover" content="ID"> + matching <item id="ID">
//     (EPUB 2 standard, used by most Calibre and Project Gutenberg files).
//  3. <guide><reference type="cover" href="..."/></guide> — older Gutenberg
//     and many hand-rolled EPUBs only declare the cover here. The reference
//     usually points at an XHTML wrapper page; we then parse that page for
//     its first <img src="..."> and resolve to the actual image.
//  4. Fallback: any file in the zip whose basename starts with "cover" and
//     looks like an image.
//
// Returns ok=false if none of the methods produce a readable image.
func ExtractEPUBCover(path string) (data []byte, ext string, ok bool) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, "", false
	}
	defer zr.Close()

	opfPath, _ := findOPFPath(&zr.Reader)
	if opfPath == "" {
		// No OPF — straight to filename fallback.
		return coverByFilename(&zr.Reader)
	}

	var opfBody []byte
	for _, f := range zr.File {
		if f.Name == opfPath {
			rc, err := f.Open()
			if err != nil {
				return nil, "", false
			}
			opfBody, _ = io.ReadAll(rc)
			rc.Close()
			break
		}
	}
	if len(opfBody) == 0 {
		return coverByFilename(&zr.Reader)
	}

	var parsed opfMetadata
	if err := xml.Unmarshal(opfBody, &parsed); err != nil {
		return coverByFilename(&zr.Reader)
	}

	opfDir := pathDir(opfPath)

	// 1. EPUB 3: <item properties="cover-image" .../>
	for _, item := range parsed.Manifest.Items {
		if containsToken(item.Properties, "cover-image") && item.Href != "" {
			if d, e, ok := readZipEntry(&zr.Reader, joinOPFPath(opfDir, item.Href)); ok {
				return d, e, true
			}
		}
	}

	// 2. EPUB 2: <meta name="cover" content="ID">
	coverID := ""
	for _, m := range parsed.Metadata.Meta {
		if strings.EqualFold(m.Name, "cover") && m.Content != "" {
			coverID = m.Content
			break
		}
	}
	if coverID != "" {
		for _, item := range parsed.Manifest.Items {
			if item.ID == coverID && item.Href != "" {
				if d, e, ok := readZipEntry(&zr.Reader, joinOPFPath(opfDir, item.Href)); ok {
					return d, e, true
				}
			}
		}
	}

	// 3. <guide><reference type="cover" href="..."/></guide>.
	// The href usually points at an XHTML wrapper, not the image itself, so
	// we read it and find the first <img src="...">.
	for _, ref := range parsed.Guide.References {
		if !strings.EqualFold(ref.Type, "cover") || ref.Href == "" {
			continue
		}
		refPath := joinOPFPath(opfDir, ref.Href)
		// Strip URL fragments — guide hrefs sometimes include "#section".
		if i := strings.Index(refPath, "#"); i >= 0 {
			refPath = refPath[:i]
		}
		// If the reference already points at an image, use it directly.
		if ext := strings.ToLower(extOf(refPath)); isImageExt(ext) {
			if d, e, ok := readZipEntry(&zr.Reader, refPath); ok {
				return d, e, true
			}
			continue
		}
		// Otherwise read the wrapper page and find the first <img src>.
		wrapper, ok := readZipEntryBytes(&zr.Reader, refPath)
		if !ok {
			continue
		}
		imgSrc := firstImageSrc(wrapper)
		if imgSrc == "" {
			continue
		}
		wrapperDir := pathDir(refPath)
		imgPath := joinOPFPath(wrapperDir, imgSrc)
		if i := strings.Index(imgPath, "#"); i >= 0 {
			imgPath = imgPath[:i]
		}
		if d, e, ok := readZipEntry(&zr.Reader, imgPath); ok {
			return d, e, true
		}
	}

	// 4. Last-ditch: any image whose filename starts with "cover".
	return coverByFilename(&zr.Reader)
}

func coverByFilename(zr *zip.Reader) ([]byte, string, bool) {
	for _, f := range zr.File {
		base := strings.ToLower(f.Name)
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		if !strings.HasPrefix(base, "cover") {
			continue
		}
		ext := extOf(base)
		if !isImageExt(ext) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		return body, ext, true
	}
	return nil, "", false
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, string, bool) {
	body, ok := readZipEntryBytes(zr, name)
	if !ok {
		return nil, "", false
	}
	return body, strings.ToLower(extOf(name)), true
}

func readZipEntryBytes(zr *zip.Reader, name string) ([]byte, bool) {
	name = strings.ReplaceAll(name, "//", "/")
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, false
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, false
			}
			return body, true
		}
	}
	// Case-insensitive fallback — some EPUBs zip with mixed case.
	lowerName := strings.ToLower(name)
	for _, f := range zr.File {
		if strings.ToLower(f.Name) == lowerName {
			rc, err := f.Open()
			if err != nil {
				return nil, false
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, false
			}
			return body, true
		}
	}
	return nil, false
}

var imgSrcRE = regexp.MustCompile(`(?i)<img[^>]*src="([^"]+)"|<image[^>]*(?:xlink:)?href="([^"]+)"`)

func firstImageSrc(html []byte) string {
	m := imgSrcRE.FindSubmatch(html)
	if len(m) == 0 {
		return ""
	}
	for i := 1; i < len(m); i++ {
		if len(m[i]) > 0 {
			return string(m[i])
		}
	}
	return ""
}

func pathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i+1]
	}
	return ""
}

func joinOPFPath(dir, href string) string {
	if strings.HasPrefix(href, "/") {
		return strings.TrimPrefix(href, "/")
	}
	combined := dir + href
	// Resolve "../" segments — Project Gutenberg files sometimes use them in
	// the guide reference (e.g. dir="OEBPS/text/" + href="../images/cover.jpg").
	parts := strings.Split(combined, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		switch p {
		case "", ".":
			// drop empty/current segments, but keep trailing slash collapse logic simple
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}

func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i:])
	}
	return ""
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg":
		return true
	}
	return false
}

func containsToken(s, token string) bool {
	for _, t := range strings.Fields(s) {
		if t == token {
			return true
		}
	}
	return false
}

// ----- PDF -----

func ExtractPDFMetadata(path string) (Metadata, error) {
	f, r, err := pdfReader.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer f.Close()

	trailer := r.Trailer()
	info := trailer.Key("Info")
	if info.IsNull() {
		return Metadata{}, nil
	}
	out := Metadata{
		Title:  strings.TrimSpace(info.Key("Title").Text()),
		Author: strings.TrimSpace(info.Key("Author").Text()),
	}
	return out, nil
}

// ----- Markdown -----

// ExtractMarkdownMetadata reads the start of the file looking for either YAML
// front-matter (`--- title: ... ---`) or the first `# H1` heading as title.
// Author isn't usually present in markdown so we leave it empty.
func ExtractMarkdownMetadata(path string) (Metadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer f.Close()

	// Read first ~8KB — enough for any reasonable front-matter or first heading.
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	head := string(buf[:n])

	if title, author := readYAMLFrontMatter(head); title != "" || author != "" {
		return Metadata{Title: title, Author: author}, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(head))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return Metadata{Title: strings.TrimSpace(strings.TrimPrefix(line, "# "))}, nil
		}
	}
	return Metadata{}, nil
}

var yamlBlockRE = regexp.MustCompile(`(?s)\A---\s*\n(.*?)\n---`)
var yamlKVRE = regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*)\s*:\s*(.+?)\s*$`)

func readYAMLFrontMatter(head string) (title, author string) {
	m := yamlBlockRE.FindStringSubmatch(head)
	if len(m) < 2 {
		return "", ""
	}
	pairs := yamlKVRE.FindAllStringSubmatch(m[1], -1)
	for _, p := range pairs {
		key := strings.ToLower(p[1])
		val := strings.Trim(strings.TrimSpace(p[2]), `"' `)
		switch key {
		case "title":
			title = val
		case "author", "creator", "by":
			author = val
		}
	}
	return title, author
}

// utility — used when the caller wants a guess at the title from the filename
// (drop the extension, replace separators with spaces, title-case-ish).
func TitleFromFilename(name string) string {
	stem := name
	if i := strings.LastIndex(name, "."); i > 0 {
		stem = name[:i]
	}
	stem = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(stem)
	stem = strings.Join(strings.Fields(stem), " ")
	return stem
}

// (sanity-check that bytes is referenced — keep it lightweight if Go's tree
// shaker complains in some flavors)
var _ = bytes.NewReader
