package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthUploadVisibilityProgressAndHighlightFlows(t *testing.T) {
	t.Parallel()

	client, baseURL := newTestClient(t)
	seedCSRFCookie(t, client, baseURL)

	signupBody := map[string]string{
		"email":    "reader@example.com",
		"password": "verysecurepass",
	}
	resp := doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/signup", signupBody, http.StatusCreated)
	var signup struct {
		User struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	decodeBody(t, resp, &signup)
	if signup.User.Email != "reader@example.com" {
		t.Fatalf("unexpected signup email: %q", signup.User.Email)
	}

	meResp := doJSON(t, client, http.MethodGet, baseURL+"/api/v1/auth/me", nil, http.StatusOK)
	var mePayload struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	decodeBody(t, meResp, &mePayload)
	if !mePayload.Authenticated {
		t.Fatal("expected authenticated user")
	}

	bookID := uploadMultipart(t, client, baseURL, map[string]string{
		"title":      "Flow Book",
		"author":     "Moco",
		"visibility": "private",
	}, "file", "flow.md", []byte("# Intro\n\nA highlightable sentence lives here.\n"))

	booksResp := doJSON(t, client, http.MethodGet, baseURL+"/api/v1/books", nil, http.StatusOK)
	var booksPayload struct {
		PrivateItems []struct {
			ID string `json:"id"`
		} `json:"privateItems"`
		PublicItems []any `json:"publicItems"`
	}
	decodeBody(t, booksResp, &booksPayload)
	if len(booksPayload.PrivateItems) != 1 || booksPayload.PrivateItems[0].ID != bookID {
		t.Fatalf("unexpected private books payload: %+v", booksPayload.PrivateItems)
	}

	doJSON(t, client, http.MethodPut, baseURL+"/api/v1/books/"+bookID+"/visibility", map[string]string{
		"visibility": "public",
	}, http.StatusOK)

	anonClient, _ := newAnonymousClient(t)
	publicResp := doJSON(t, anonClient, http.MethodGet, baseURL+"/api/v1/books/public", nil, http.StatusOK)
	var publicPayload struct {
		Items []struct {
			ID         string `json:"id"`
			Visibility string `json:"visibility"`
		} `json:"items"`
	}
	decodeBody(t, publicResp, &publicPayload)
	if len(publicPayload.Items) != 1 || publicPayload.Items[0].ID != bookID || publicPayload.Items[0].Visibility != "public" {
		t.Fatalf("unexpected public payload: %+v", publicPayload.Items)
	}

	readerResp, err := anonClient.Get(baseURL + "/books/" + bookID + "/read")
	if err != nil {
		t.Fatalf("reader page request failed: %v", err)
	}
	defer readerResp.Body.Close()
	readerHTML, _ := io.ReadAll(readerResp.Body)
	if readerResp.StatusCode != http.StatusOK {
		t.Fatalf("reader page status = %d, want %d", readerResp.StatusCode, http.StatusOK)
	}
	if !strings.Contains(string(readerHTML), `data-reader-kind="md"`) {
		t.Fatal("expected markdown reader route to render in-app reader")
	}

	doJSON(t, client, http.MethodPut, baseURL+"/api/v1/books/"+bookID+"/progress", map[string]any{
		"locator":         "intro-1",
		"progressPercent": 42.5,
	}, http.StatusOK)

	progressResp := doJSON(t, client, http.MethodGet, baseURL+"/api/v1/books/"+bookID+"/progress", nil, http.StatusOK)
	var progressPayload struct {
		Progress struct {
			Locator         string  `json:"locator"`
			ProgressPercent float64 `json:"progressPercent"`
		} `json:"progress"`
	}
	decodeBody(t, progressResp, &progressPayload)
	if progressPayload.Progress.Locator != "intro-1" || progressPayload.Progress.ProgressPercent != 42.5 {
		t.Fatalf("unexpected progress payload: %+v", progressPayload.Progress)
	}

	highlightResp := doJSON(t, client, http.MethodPost, baseURL+"/api/v1/books/"+bookID+"/highlights", map[string]string{
		"locator":      "intro-1",
		"selectedText": "A highlightable sentence lives here.",
		"color":        "amber",
	}, http.StatusCreated)
	var highlightPayload struct {
		Highlight struct {
			ID           string `json:"id"`
			Locator      string `json:"locator"`
			SelectedText string `json:"selectedText"`
		} `json:"highlight"`
	}
	decodeBody(t, highlightResp, &highlightPayload)
	if highlightPayload.Highlight.Locator != "intro-1" {
		t.Fatalf("unexpected highlight locator: %+v", highlightPayload.Highlight)
	}

	highlightsResp := doJSON(t, client, http.MethodGet, baseURL+"/api/v1/books/"+bookID+"/highlights", nil, http.StatusOK)
	var highlightsPayload struct {
		Items []struct {
			ID           string `json:"id"`
			SelectedText string `json:"selectedText"`
		} `json:"items"`
	}
	decodeBody(t, highlightsResp, &highlightsPayload)
	if len(highlightsPayload.Items) != 1 || highlightsPayload.Items[0].ID == "" {
		t.Fatalf("unexpected highlights payload: %+v", highlightsPayload.Items)
	}
}

func TestUnsafeRequestsRequireCSRFTokens(t *testing.T) {
	t.Parallel()

	client, baseURL := newTestClient(t)

	reqBody, _ := json.Marshal(map[string]string{
		"email":    "blocked@example.com",
		"password": "verysecurepass",
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/auth/signup", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestEPUBAndPDFReaderRoutesRenderInAppReaders(t *testing.T) {
	t.Parallel()

	client, baseURL := newTestClient(t)
	seedCSRFCookie(t, client, baseURL)
	doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/signup", map[string]string{
		"email":    "formats@example.com",
		"password": "verysecurepass",
	}, http.StatusCreated)

	epubID := uploadMultipart(t, client, baseURL, map[string]string{
		"title":      "EPUB Book",
		"visibility": "public",
	}, "file", "sample.epub", []byte("epub placeholder"))
	pdfID := uploadMultipart(t, client, baseURL, map[string]string{
		"title":      "PDF Book",
		"visibility": "public",
	}, "file", "sample.pdf", []byte("%PDF-1.4 placeholder"))

	assertReaderContains(t, client, baseURL+"/books/"+epubID+"/read", `data-reader-kind="epub"`)
	assertReaderContains(t, client, baseURL+"/books/"+epubID+"/read", `epub.min.js`)
	assertReaderContains(t, client, baseURL+"/books/"+pdfID+"/read", `data-reader-kind="pdf"`)
	assertReaderContains(t, client, baseURL+"/books/"+pdfID+"/read", `pdf.min.mjs`)
}

func TestInspectPDFIncludesConversionDiagnostics(t *testing.T) {
	t.Parallel()

	client, baseURL := newTestClient(t)
	seedCSRFCookie(t, client, baseURL)
	doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/signup", map[string]string{
		"email":    "inspect@example.com",
		"password": "verysecurepass",
	}, http.StatusCreated)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "blank.pdf")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(minimalPDFBytes()); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/books/inspect", &body)
	if err != nil {
		t.Fatalf("new inspect request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken(t, client, baseURL))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("inspect request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("inspect status = %d, want %d, body=%s", resp.StatusCode, http.StatusOK, string(bodyBytes))
	}

	var payload struct {
		Title      string `json:"title"`
		Format     string `json:"format"`
		Conversion struct {
			Profile        string `json:"profile"`
			PageCount      int    `json:"pageCount"`
			LooksMathHeavy bool   `json:"looksMathHeavy"`
		} `json:"conversion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode inspect body: %v", err)
	}
	if payload.Format != "pdf" {
		t.Fatalf("format = %q, want pdf", payload.Format)
	}
	if payload.Conversion.Profile == "" {
		t.Fatal("expected conversion profile in inspect response")
	}
	if payload.Conversion.PageCount < 1 {
		t.Fatalf("pageCount = %d, want >= 1", payload.Conversion.PageCount)
	}
}

func assertReaderContains(t *testing.T, client *http.Client, targetURL, needle string) {
	t.Helper()
	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("get %s: %v", targetURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get %s status = %d", targetURL, resp.StatusCode)
	}
	if !strings.Contains(string(body), needle) {
		t.Fatalf("expected %q in response for %s", needle, targetURL)
	}
}

func newTestClient(t *testing.T) (*http.Client, string) {
	t.Helper()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "moco.sqlite")
	srv := New(Config{
		DataDir:       dataDir,
		DBPath:        dbPath,
		CookieName:    "moco_session",
		SecureCookies: false,
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}, ts.URL
}

func newAnonymousClient(t *testing.T) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}, ""
}

func seedCSRFCookie(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		t.Fatalf("seed csrf cookie: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed csrf status = %d", resp.StatusCode)
	}
	if csrfToken(t, client, baseURL) == "" {
		t.Fatal("csrf cookie was not set")
	}
}

func csrfToken(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == csrfCookieName {
			return cookie.Value
		}
	}
	return ""
}

func doJSON(t *testing.T, client *http.Client, method, targetURL string, payload any, wantStatus int) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, targetURL, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !isSafeMethod(method) {
		req.Header.Set("X-CSRF-Token", csrfToken(t, client, targetURL))
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if resp.StatusCode != wantStatus {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("%s %s status = %d, want %d, body=%s", method, targetURL, resp.StatusCode, wantStatus, string(bodyBytes))
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, target any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("decode body: %v", err)
	}
}

func uploadMultipart(t *testing.T, client *http.Client, baseURL string, fields map[string]string, fileField, filename string, contents []byte) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(contents); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/books/upload", &body)
	if err != nil {
		t.Fatalf("new upload request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken(t, client, baseURL))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d, want %d, body=%s", resp.StatusCode, http.StatusCreated, string(bodyBytes))
	}

	var payload struct {
		Book struct {
			ID string `json:"id"`
		} `json:"book"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode upload body: %v", err)
	}
	if payload.Book.ID == "" {
		t.Fatal("upload response missing book id")
	}
	return payload.Book.ID
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func minimalPDFBytes() []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for i, obj := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(offsets))
	buf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefOffset)
	return buf.Bytes()
}
