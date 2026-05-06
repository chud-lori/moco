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
	} `xml:"metadata"`
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
