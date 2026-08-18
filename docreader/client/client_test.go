package client

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"roche.local/knowledge-agent-platform/docreader/proto"
)

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.Println("INFO: Initializing DocReader client tests")
}

func requireIntegrationClient(t *testing.T) *Client {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv("DOCREADER_TEST_ADDR"))
	if addr == "" {
		t.Skip("set DOCREADER_TEST_ADDR to run DocReader integration tests")
	}

	client, err := NewClient(addr)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client: %v", err)
		}
	})
	client.SetDebug(true)
	return client
}

func TestReadURL(t *testing.T) {
	client := requireIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	startTime := time.Now()
	resp, err := client.Read(
		ctx,
		&proto.ReadRequest{
			Url:   "https://example.com",
			Title: "test",
		},
	)
	log.Printf("INFO: Read(URL) completed in %v", time.Since(startTime))

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("Read returned error: %s", resp.Error)
	}
	if resp.MarkdownContent == "" {
		t.Error("Expected non-empty markdown content")
	}
	log.Printf("INFO: content_len=%d, images=%d", len(resp.MarkdownContent), len(resp.ImageRefs))
}

func TestReadFile(t *testing.T) {
	client := requireIntegrationClient(t)

	fileContent, err := os.ReadFile("../testdata/test.md")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	startTime := time.Now()
	resp, err := client.Read(
		ctx,
		&proto.ReadRequest{
			FileContent: fileContent,
			FileName:    "test.md",
			FileType:    "md",
		},
	)
	log.Printf("INFO: Read(file) completed in %v", time.Since(startTime))

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("Read returned error: %s", resp.Error)
	}
	if resp.MarkdownContent == "" {
		t.Error("Expected non-empty markdown content")
	}

	imageRefs := GetImageRefsFromResponse(resp)
	log.Printf("INFO: content_len=%d, images=%d", len(resp.MarkdownContent), len(imageRefs))
}
