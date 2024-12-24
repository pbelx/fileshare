package main

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// FileInfo represents the metadata of an uploaded file
type FileInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Type       string    `json:"type"`
	UploadDate time.Time `json:"uploadDate"`
	Path       string    `json:"-"` // Private field, not exposed in JSON
}

var (
	uploadDir    = "uploads"
	maxFileSize  = int64(10 << 20) // 10 MB
	allowedTypes = map[string]bool{
		"image/jpeg":         true,
		"image/png":          true,
		"image/gif":          true,
		"application/pdf":    true,
		"text/plain":         true,
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	}
)

func main() {
	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatal(err)
	}

	router := setupRouter()
	router.Run(":8080")
}

func setupRouter() *gin.Engine {
	router := gin.Default()

	// Configure CORS
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"} // Update with your frontend URL
	config.AllowMethods = []string{"GET", "POST", "DELETE"}
	config.AllowHeaders = []string{"Origin", "Content-Type"}
	router.Use(cors.New(config))

	// Routes
	router.MaxMultipartMemory = 8 << 20 // 8 MiB
	router.POST("/upload", handleFileUpload)
	router.GET("/files", listFiles)
	router.GET("/files/:id/download", downloadFile)
	router.DELETE("/files/:id", deleteFile)

	return router
}

func handleFileUpload(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No files provided"})
		return
	}

	uploadedFiles := make([]FileInfo, 0)

	for _, file := range files {
		if err := validateFile(file); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		fileInfo, err := saveFile(c, file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
			return
		}

		uploadedFiles = append(uploadedFiles, *fileInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Files uploaded successfully",
		"files":   uploadedFiles,
	})
}

func validateFile(file *multipart.FileHeader) error {
	// Check file size
	if file.Size > maxFileSize {
		return fmt.Errorf("file %s is too large (max %d MB)", file.Filename, maxFileSize/(1<<20))
	}

	// Check file type
	contentType := file.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		return fmt.Errorf("file type %s is not allowed", contentType)
	}

	return nil
}

func saveFile(c *gin.Context, file *multipart.FileHeader) (*FileInfo, error) {
	// Generate unique filename
	ext := filepath.Ext(file.Filename)
	id := uuid.New().String()
	filename := id + ext

	// Save file to disk
	dst := filepath.Join(uploadDir, filename)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return nil, err
	}

	if err := c.SaveUploadedFile(file, dst); err != nil {
		return nil, err
	}

	// Create file info
	fileInfo := &FileInfo{
		ID:         id,
		Name:       file.Filename,
		Size:       file.Size,
		Type:       strings.Split(file.Header.Get("Content-Type"), "/")[0],
		UploadDate: time.Now(),
		Path:       dst,
	}

	return fileInfo, nil
}

func listFiles(c *gin.Context) {
	var files []FileInfo
	err := filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			files = append(files, FileInfo{
				ID:         id,
				Name:       info.Name(),
				Size:       info.Size(),
				Type:       getFileType(path),
				UploadDate: info.ModTime(),
				Path:       path,
			})
		}
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list files"})
		return
	}

	c.JSON(http.StatusOK, files)
}

func downloadFile(c *gin.Context) {
	id := c.Param("id")
	var filePath string

	// Find the file with matching ID
	err := filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), id) {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil || filePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	c.File(filePath)
}

func deleteFile(c *gin.Context) {
	id := c.Param("id")
	var filePath string

	// Find the file with matching ID
	err := filepath.Walk(uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), id) {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil || filePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	if err := os.Remove(filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "File deleted successfully"})
}

func getFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		return "image"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text"
	case ".doc", ".docx":
		return "document"
	default:
		return "unknown"
	}
}
