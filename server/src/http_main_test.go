package main

import (
	"bytes"
	"net/http/httptest"
	"path"
	"testing"
)

func TestMainPageTitleUsesWebsiteNameInDevMode(t *testing.T) {
	researchTestInit(t)
	versionPath = path.Join(projectPath, "public", "js", "bundles", "version.txt")
	isDev = true

	response := httptest.NewRecorder()
	httpServeTemplate(response, &TemplateData{
		Title:  "Main",
		Domain: "localhost",
		IsDev:  isDev,
	}, "main")

	body := response.Body.Bytes()
	if bytes.Contains(body, []byte("<title>Test</title>")) {
		t.Fatalf("main page title still uses the dev placeholder: %q", titleHTML(body))
	}
	if !bytes.Contains(body, []byte("<title>Hanabi</title>")) {
		t.Fatalf("main page title does not use WebsiteName: %q", titleHTML(body))
	}
}

func titleHTML(body []byte) []byte {
	start := bytes.Index(body, []byte("<title>"))
	end := bytes.Index(body, []byte("</title>"))
	if start == -1 || end == -1 || end < start {
		return nil
	}
	return body[start : end+len("</title>")]
}
