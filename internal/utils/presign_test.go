package utils

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignFileURL_RoundTrip(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "rochekap-test-aes-key-32bytes!!!")

	baseURL := "https://rochekap.example.com"
	filePath := "local://1/abc/img.png"
	var knowledgeDomainID uint64 = 1

	signed, err := SignFileURL(baseURL, filePath, knowledgeDomainID, 1*time.Hour)
	require.NoError(t, err)
	assert.Contains(t, signed, "https://rochekap.example.com/api/v1/files/presigned")
	assert.Contains(t, signed, "file_path=")
	assert.Contains(t, signed, "knowledge_domain_id=1")
	assert.Contains(t, signed, "sig=")
}

func TestSignFileURL_NoKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")

	_, err := SignFileURL("https://example.com", "local://1/img.png", 1, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SYSTEM_AES_KEY")
}

func TestVerifyFileURLSig_Valid(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "rochekap-test-aes-key-32bytes!!!")

	filePath := "local://42/knowledge/img.jpg"
	var knowledgeDomainID uint64 = 42
	key := getPresignKey()
	require.NotNil(t, key)

	expires := time.Now().Add(1 * time.Hour).Unix()
	sig := signPayload(key, filePath, knowledgeDomainID, expires)

	assert.True(t, VerifyFileURLSig(filePath, knowledgeDomainID, strconv.FormatInt(expires, 10), sig))
}

func TestVerifyFileURLSig_Expired(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "rochekap-test-aes-key-32bytes!!!")

	filePath := "local://1/img.png"
	var knowledgeDomainID uint64 = 1
	key := getPresignKey()
	require.NotNil(t, key)

	expires := time.Now().Add(-1 * time.Hour).Unix() // already expired
	sig := signPayload(key, filePath, knowledgeDomainID, expires)

	assert.False(t, VerifyFileURLSig(filePath, knowledgeDomainID, strconv.FormatInt(expires, 10), sig))
}

func TestVerifyFileURLSig_Tampered(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "rochekap-test-aes-key-32bytes!!!")

	filePath := "local://1/img.png"
	var knowledgeDomainID uint64 = 1
	key := getPresignKey()
	require.NotNil(t, key)

	expires := time.Now().Add(1 * time.Hour).Unix()
	sig := signPayload(key, filePath, knowledgeDomainID, expires)
	expiresStr := strconv.FormatInt(expires, 10)

	// Tamper with file path
	assert.False(t, VerifyFileURLSig("local://1/other.png", knowledgeDomainID, expiresStr, sig))
	// Tamper with knowledgeDomain ID
	assert.False(t, VerifyFileURLSig(filePath, 999, expiresStr, sig))
	// Tamper with signature
	assert.False(t, VerifyFileURLSig(filePath, knowledgeDomainID, expiresStr, "deadbeef"))
}

func TestVerifyFileURLSig_NoKey(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", "")

	assert.False(t, VerifyFileURLSig("local://1/img.png", 1, "99999999999", "abc"))
}

func TestValidateStoragePathKnowledgeDomain(t *testing.T) {
	assert.NoError(t, ValidateStoragePathKnowledgeDomain("local://42/knowledge/file.pdf", 42))
	assert.Error(t, ValidateStoragePathKnowledgeDomain("local://7/knowledge/file.pdf", 42))
	assert.Error(t, ValidateStoragePathKnowledgeDomain("local://docs/example.txt", 42))
}

func TestParseKnowledgeDomainIDFromStoragePath(t *testing.T) {
	tests := []struct {
		path string
		want uint64
	}{
		{"local://1/abc/img.png", 1},
		{"local://42/knowledge/file.pdf", 42},
		{"minio://bucket/1/abc/img.png", 1},
		{"s3://bucket/rochekap/1/abc/img.png", 1},
		{"cos://bucket/region/prefix/1/abc/img.png", 1},
		{"tos://bucket/1/abc/img.png", 1},
		{"oss://bucket/rochekap/1/abc/img.png", 1},
		{"https://example.com/img.png", 0},
		{"invalid", 0},
		{"local://exports/file.csv", 0},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := ParseKnowledgeDomainIDFromStoragePath(tt.path)
			assert.Equal(t, tt.want, got, "ParseKnowledgeDomainIDFromStoragePath(%q)", tt.path)
		})
	}
}
