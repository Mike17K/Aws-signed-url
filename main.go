package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-chi/chi"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Failed to load .env file")
		os.Exit(1)
	}

	r := chi.NewRouter()

	r.Post("/get-upload-url", GetUploadURLHandler)

	http.ListenAndServe(":3000", r)
	fmt.Println("Server started at http://localhost:3000")
}

// Route GetUploadURL

type GeneratePresignedURLBody struct {
	ContentLength int64 `json:"content_length"`
}

type GeneratePresignedURLResponse struct {
	Method         string    `json:"method"`
	PreAssignedURL string    `json:"pre_assigned_url"`
	ExpirationTime time.Time `json:"expiration_time"`
	FileName       string    `json:"file_name"`
	Host           string    `json:"host"`
	Details        []string  `json:"details"`
	ObjectUrl      string    `json:"object_url"`
}

func GetUploadURLHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the request body
	var body GeneratePresignedURLBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		// Send bad request response
		SendResponse(w, Error("invalid request body", err), http.StatusBadRequest)
		return
	}
	const MAX_UPLOAD_SIZE int64 = 1 * 1024 * 1024 // 1 MB
	if body.ContentLength <= 0 || body.ContentLength > MAX_UPLOAD_SIZE {
		SendResponse(w, Error("invalid content length", nil), http.StatusBadRequest)
		return
	}
	r.Body.Close()
	// Validations - End

	// Generate pre-signed URL
	fileName := fmt.Sprintf("%s.png", time.Now().Format("2006-01-02-15-04-05"))
	uploadTimeout := 10 * time.Minute
	bucketName := os.Getenv("AWS_BUCKET")
	PreAssignedURL, err := GeneratePresignedURL(GeneratePresignedURLParam{
		FileName:      fileName,
		Timout:        uploadTimeout,
		ContentLength: body.ContentLength,
		Bucket:        bucketName,
		ContentType:   "image/png",
	})
	if err != nil {
		SendResponse(w, Error("failed to create AWS session", err), http.StatusInternalServerError)
		return
	}

	// Send the response
	SendResponse(w, Success("pre-signed URL generated", GeneratePresignedURLResponse{
		Method:         PreAssignedURL.Method,
		PreAssignedURL: PreAssignedURL.PreAssignedURL,
		ExpirationTime: PreAssignedURL.ExpirationTime,
		FileName:       PreAssignedURL.FileName,
		Host:           PreAssignedURL.Host,
		Details:        PreAssignedURL.Details,
		ObjectUrl:      PreAssignedURL.ObjectUrl,
	}), http.StatusOK)

}

// s3service

type GeneratePresignedURLParam struct {
	FileName      string
	Timout        time.Duration
	ContentLength int64
	Bucket        string
	ContentType   string
}

func GeneratePresignedURL(param GeneratePresignedURLParam) (GeneratePresignedURLResponse, error) {

	var res GeneratePresignedURLResponse

	// aws s3
	region := os.Getenv("AWS_REGION")

	// Create a new session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		return res, fmt.Errorf("failed to create AWS session")
	}

	// Create S3 service client
	svc := s3.New(sess)

	// Set the expiration for the pre-signed URL
	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket:        aws.String(param.Bucket),
		Key:           aws.String(param.FileName),
		ContentType:   aws.String(param.ContentType),
		ContentLength: aws.Int64(param.ContentLength),
	})

	urlStr, err := req.Presign(param.Timout)
	if err != nil {
		return res, fmt.Errorf("failed to sign request")
	}

	// Return the pre-signed URL
	res.Method = "PUT"
	res.PreAssignedURL = urlStr
	res.FileName = param.FileName
	res.ExpirationTime = time.Now().Add(param.Timout)
	res.Host = fmt.Sprintf("%s.s3.amazonaws.com", param.Bucket)
	res.Details = []string{
		"Use the pre-signed URL to upload the file",
		fmt.Sprintf("The URL will expire after %d minutes", param.Timout),
		fmt.Sprintf("The maximum upload size is %d bytes", param.ContentLength),
	}
	res.ObjectUrl = fmt.Sprintf("https://%s/%s", res.Host, param.FileName)

	return res, nil
}

// Helper functions

func SendResponse(w http.ResponseWriter, response interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

func Error(message string, err error) map[string]interface{} {
	res := map[string]interface{}{
		"success": false,
		"message": message,
	}
	if err != nil {
		res["error"] = err.Error()
	}
	return res
}

func Success(message string, data interface{}) map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"message": message,
		"data":    data,
	}
}
