package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gabriel-vasile/mimetype"
)

func getS3Client() (*s3.Client, string, string, error) {
	bucket := common.OptionMap["S3Bucket"]
	endpoint := common.OptionMap["S3Endpoint"]
	s3AK := common.OptionMap["S3AK"]
	s3SK := common.OptionMap["S3SK"]
	s3Region := common.OptionMap["S3Region"]
	if bucket == "" || endpoint == "" || s3AK == "" || s3SK == "" || s3Region == "" {
		return nil, bucket, endpoint, fmt.Errorf("S3 configuration is incomplete")
	}
	s3Client := s3.New(s3.Options{
		Region:      s3Region,
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(s3AK, s3SK, "")),
	})
	return s3Client, bucket, endpoint, nil
}

// UploadOnceToS3 uploads a file (base64/data-uri or http url) to S3 exactly once and
// returns the underlying error on failure. Unlike SimpleUploadToS3/UploadFileToS3 (which
// retry internally and mask the root cause with a generic message), this surfaces the real
// error so callers can log the actual failure reason.
func UploadOnceToS3(ctx context.Context, file string) (string, error) {
	s3Client, bucket, endpoint, err := getS3Client()
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(file, "http") {
		return UploadeFromUrlToS3(ctx, s3Client, bucket, endpoint, file)
	}
	return UploadBase64ToS3(ctx, s3Client, bucket, endpoint, file)
}

func SimpleUploadToS3(ctx context.Context, file string) (string, error) {
	// bucket := common.OptionMap["S3Bucket"]
	// endpoint := common.OptionMap["S3Endpoint"]
	// s3AK := common.OptionMap["S3AK"]
	// s3SK := common.OptionMap["S3SK"]
	// s3Region := common.OptionMap["S3Region"]
	// if bucket == "" || endpoint == "" || s3AK == "" || s3SK == "" || s3Region == "" {
	// 	return "", fmt.Errorf("S3 configuration is incomplete")
	// }
	// s3Client := s3.New(s3.Options{
	// 	Region:      s3Region,
	// 	Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(s3AK, s3SK, "")),
	// })
	s3Client, bucket, endpoint, err := getS3Client()
	if err != nil {
		return "", err
	}
	return UploadFileToS3(ctx, s3Client, bucket, endpoint, file)
}

func UploadIOReaderToS3(ctx context.Context, req *http.Response) (string, error) {

	mimeType := req.Header.Get("Content-Type")
	// mimeType, err := mimetype.DetectReader(req.Body)
	// if err != nil {
	// 	return "", fmt.Errorf("failed to detect mime type: %w", err)
	// }
	//ext := mimeType.Extension()
	//ext := ".mp4"
	ext := mime.TypeByExtension(mimeType)
	fileType := strings.Split(mimeType, "/")[0]
	key := fmt.Sprintf("%ss/%d-%s%s", fileType, time.Now().UnixNano(), common.GetRandomString(10), ext)
	s3Client, bucket, endpoint, err := getS3Client()
	if err != nil {
		return "", err
	}

	uploader := manager.NewUploader(s3Client)
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          req.Body,
		ContentType:   aws.String(mimeType),
		ContentLength: aws.Int64(int64(req.ContentLength)),
	})

	// _, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
	// 	Bucket:      aws.String(bucket),
	// 	Key:         aws.String(key),
	// 	Body:        req.Body,
	// 	ContentType:   aws.String(mimeType),
	// 	ContentLength: aws.Int64(int64(req.ContentLength)),
	// })

	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}
	return fmt.Sprintf("https://%s.%s/%s", bucket, endpoint, key), nil
}

func UploadFileToS3(ctx context.Context, s3Client *s3.Client, bucket, endpoint, file string) (string, error) {
	// Generate a unique filename
	for i := 0; i < 3; i++ {
		if strings.HasPrefix(file, "http") {
			url, err := UploadeFromUrlToS3(ctx, s3Client, bucket, endpoint, file)
			if err == nil {
				return url, nil
			}
		} else {
			url, err := UploadBase64ToS3(ctx, s3Client, bucket, endpoint, file)
			if err == nil {
				return url, nil
			}
		}
	}
	return "", fmt.Errorf("failed to upload file to S3")
}

// UploadBase64ImageToS3 uploads a base64 encoded image to S3 and returns the URL
func UploadBase64ToS3(ctx context.Context, s3Client *s3.Client, bucket, endpoint, base64Data string) (string, error) {
	// Remove data URL prefix if present
	if len(base64Data) > 0 && base64Data[0] == 'd' {
		// Check if it starts with "data:image/" or "data:audio/"
		isImage := strings.HasPrefix(base64Data, "data:image/")
		isAudio := strings.HasPrefix(base64Data, "data:audio/")
		isVideo := strings.HasPrefix(base64Data, "data:video/")
		if len(base64Data) > 11 && (isImage || isAudio || isVideo) {
			// Find the comma that separates the metadata from the base64 data
			for i := 11; i < len(base64Data); i++ {
				if base64Data[i] == ',' {
					base64Data = base64Data[i+1:]
					break
				}
			}
		}
	}

	// Decode base64 data
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 data: %w", err)
	}
	// get mime type from base64 data
	mimeType := mimetype.Detect(imageData)
	mimeTypeStr := mimeType.String()
	fileType := strings.Split(mimeTypeStr, "/")[0]
	// Generate a unique filename
	filename := fmt.Sprintf("%ss/%d-%s%s", fileType, time.Now().UnixNano(), common.GetRandomString(10), mimeType.Extension())

	// Upload to S3
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(imageData),
		ContentType: aws.String(mimeTypeStr),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Generate the URL
	url := fmt.Sprintf("https://%s.%s/%s", bucket, endpoint, filename)
	logger.LogInfo(ctx, fmt.Sprintf("Successfully uploaded file to S3: %s", url))
	return url, nil
}

func UploadeFromUrlToS3(ctx context.Context, s3Client *s3.Client, bucket, endpoint, url string) (string, error) {
	// Download the image from the URL
	response, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download image from URL: %w", err)
	}
	defer response.Body.Close()

	// get mime type from response
	statusCode := response.StatusCode
	if statusCode != 200 {
		return "", fmt.Errorf("failed to get image type from URL")
	}

	// get image data from response
	imageData, err := io.ReadAll(response.Body)
	mimeType := mimetype.Detect(imageData)
	filename := fmt.Sprintf("images/%d-%s%s", time.Now().UnixNano(), common.GetRandomString(10), mimeType.Extension())
	if err != nil {
		return "", fmt.Errorf("failed to read image data from URL: %w", err)
	}

	// upload image data to S3
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(imageData),
		ContentType: aws.String(mimeType.String()),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload image to S3: %w", err)
	}

	// generate the url
	imageUrl := fmt.Sprintf("https://%s.%s/%s", bucket, endpoint, filename)
	logger.LogInfo(ctx, fmt.Sprintf("Successfully uploaded file to S3: %s", url))
	return imageUrl, nil
}
