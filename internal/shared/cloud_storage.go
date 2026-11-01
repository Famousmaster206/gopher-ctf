package shared

import (
	"crypto"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type serviceAccountCredentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey string `json:"private_key"`
	TokenURI   string `json:"token_uri"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// UploadChallengeFile streams an uploaded challenge attachment to Google Cloud
// Storage without creating a persistent local file.
func UploadChallengeFile(c *gin.Context, challengeID uint, file *multipart.FileHeader) error {
	if file == nil {
		return errors.New("missing challenge file")
	}
	bucket := strings.TrimSpace(os.Getenv("GCS_BUCKET"))
	bucket = strings.TrimSuffix(strings.TrimPrefix(bucket, "gs://"), "/")
	credentialsJSON := os.Getenv("GCS_CREDENTIALS_JSON")
	if bucket == "" || strings.TrimSpace(credentialsJSON) == "" {
		return errors.New("Google Cloud Storage is not configured")
	}

	var credentials serviceAccountCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return errors.New("invalid Google Cloud Storage credentials")
	}
	if credentials.ClientEmail == "" || credentials.PrivateKey == "" {
		return errors.New("incomplete Google Cloud Storage credentials")
	}
	if credentials.TokenURI == "" {
		credentials.TokenURI = "https://oauth2.googleapis.com/token"
	}

	token, err := storageAccessToken(c.Request.Context(), credentials)
	if err != nil {
		return err
	}

	name := ChallengeFilename(file.Filename)
	if name == "." || name == "" {
		return errors.New("invalid challenge filename")
	}
	objectName := fmt.Sprintf("challenges/%d/%s", challengeID, name)
	return uploadStorageObject(c.Request.Context(), token, bucket, objectName, file)
}

func ChallengeFilename(filename string) string {
	return filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
}

func DownloadChallengeFile(c *gin.Context, challengeID uint, filename string) error {
	bucket := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(os.Getenv("GCS_BUCKET")), "gs://"), "/")
	credentialsJSON := os.Getenv("GCS_CREDENTIALS_JSON")
	if bucket == "" || strings.TrimSpace(credentialsJSON) == "" {
		return errors.New("Google Cloud Storage is not configured")
	}
	var credentials serviceAccountCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return errors.New("invalid Google Cloud Storage credentials")
	}
	if credentials.TokenURI == "" {
		credentials.TokenURI = "https://oauth2.googleapis.com/token"
	}
	token, err := storageAccessToken(c.Request.Context(), credentials)
	if err != nil {
		return err
	}
	objectName := fmt.Sprintf("challenges/%d/%s", challengeID, ChallengeFilename(filename))
	endpoint := "https://storage.googleapis.com/storage/v1/b/" + url.PathEscape(bucket) + "/o/" + url.PathEscape(objectName) + "?alt=media"
	request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Google Cloud Storage download failed with status %s", response.Status)
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	downloadName := strings.NewReplacer("\"", "", "\r", "", "\n", "").Replace(ChallengeFilename(filename))
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", `attachment; filename="`+downloadName+`"; filename*=UTF-8''`+url.PathEscape(downloadName))
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	if response.ContentLength >= 0 {
		c.Header("Content-Length", fmt.Sprintf("%d", response.ContentLength))
	}
	_, err = io.Copy(c.Writer, response.Body)
	return err
}

func storageAccessToken(ctx context.Context, credentials serviceAccountCredentials) (string, error) {
	block, _ := pem.Decode([]byte(credentials.PrivateKey))
	if block == nil {
		return "", errors.New("invalid Google Cloud private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("Google Cloud private key is not RSA")
	}
	now := time.Now().Unix()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]interface{}{
		"iss": credentials.ClientEmail,
		"scope": "https://www.googleapis.com/auth/devstorage.read_write",
		"aud": credentials.TokenURI,
		"iat": now,
		"exp": now + 3600,
	})
	encodedClaims := base64.RawURLEncoding.EncodeToString(claims)
	unsigned := header + "." + encodedClaims
	digest := sha256Sum([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(nil, privateKey, crypto.SHA256, digest)
	if err != nil {
		return "", err
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, credentials.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Google Cloud token request failed with status %s", response.Status)
	}
	var token tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil || token.AccessToken == "" {
		return "", errors.New("Google Cloud token response was invalid")
	}
	return token.AccessToken, nil
}

func sha256Sum(data []byte) []byte {
	// The standard library hash implementation is kept in a small helper so
	// the upload path remains easy to test and audit.
	h := sha256.New()
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func uploadStorageObject(ctx context.Context, token, bucket, objectName string, file *multipart.FileHeader) error {
	opened, err := file.Open()
	if err != nil {
		return err
	}
	defer opened.Close()
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	metadata, _ := json.Marshal(map[string]string{"name": objectName, "contentType": contentType})
	boundary := "gopher-ctf-upload"
	prefix := fmt.Sprintf("--%s\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n%s\r\n--%s\r\nContent-Type: %s\r\nContent-Transfer-Encoding: binary\r\n\r\n", boundary, metadata, boundary, contentType)
	suffix := fmt.Sprintf("\r\n--%s--\r\n", boundary)
	reader, writer := io.Pipe()
	go func() {
		_, writeErr := io.WriteString(writer, prefix)
		if writeErr == nil {
			_, writeErr = io.Copy(writer, opened)
		}
		if writeErr == nil {
			_, writeErr = io.WriteString(writer, suffix)
		}
		_ = writer.CloseWithError(writeErr)
	}()

	endpoint := "https://storage.googleapis.com/upload/storage/v1/b/" + url.PathEscape(bucket) + "/o?uploadType=multipart"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "multipart/related; boundary="+boundary)
	request.ContentLength = int64(len(prefix)) + file.Size + int64(len(suffix))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Google Cloud Storage upload failed with status %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	return nil
}
