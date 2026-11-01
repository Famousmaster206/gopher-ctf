package shared

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/option"
)

func storageClient(c *gin.Context) (*storage.Client, string, error) {
	bucket := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(os.Getenv("GCS_BUCKET")), "gs://"), "/")
	credentialsJSON := strings.TrimSpace(os.Getenv("GCS_CREDENTIALS_JSON"))
	if bucket == "" || credentialsJSON == "" {
		return nil, "", errors.New("Google Cloud Storage is not configured")
	}
	client, err := storage.NewClient(c.Request.Context(), option.WithCredentialsJSON([]byte(credentialsJSON)))
	if err != nil {
		return nil, "", errors.New("Google Cloud Storage client could not be initialized")
	}
	return client, bucket, nil
}

// UploadChallengeFile streams an uploaded challenge attachment directly to GCS.
func UploadChallengeFile(c *gin.Context, challengeID uint, file *multipart.FileHeader) error {
	if file == nil {
		return errors.New("missing challenge file")
	}
	client, bucket, err := storageClient(c)
	if err != nil {
		return err
	}
	defer client.Close()

	name := ChallengeFilename(file.Filename)
	if name == "." || name == "" {
		return errors.New("invalid challenge filename")
	}
	opened, err := file.Open()
	if err != nil {
		return err
	}
	defer opened.Close()

	object := client.Bucket(bucket).Object(fmt.Sprintf("challenges/%d/%s", challengeID, name))
	writer := object.NewWriter(c.Request.Context())
	writer.ContentType = file.Header.Get("Content-Type")
	if writer.ContentType == "" {
		writer.ContentType = "application/octet-stream"
	}
	if _, err := io.Copy(writer, opened); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func ChallengeFilename(filename string) string {
	return filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
}

// DownloadChallengeFile streams a private GCS object through the application.
func DownloadChallengeFile(c *gin.Context, challengeID uint, filename string) error {
	client, bucket, err := storageClient(c)
	if err != nil {
		return err
	}
	defer client.Close()

	name := ChallengeFilename(filename)
	reader, err := client.Bucket(bucket).Object(fmt.Sprintf("challenges/%d/%s", challengeID, name)).NewReader(c.Request.Context())
	if err != nil {
		return err
	}
	defer reader.Close()

	contentType := reader.Attrs.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	downloadName := strings.NewReplacer("\"", "", "\r", "", "\n", "").Replace(name)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", `attachment; filename="`+downloadName+`"`)
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Length", fmt.Sprintf("%d", reader.Attrs.Size))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, no-store")
	_, err = io.Copy(c.Writer, reader)
	return err
}
