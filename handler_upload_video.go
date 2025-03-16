package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	var maxMemory int64
	maxMemory = 10 << 30

	err = r.ParseMultipartForm(maxMemory)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
		return
	}

	videoFile, header, err := r.FormFile("video")
	defer videoFile.Close()
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file", err)
		return
	}

	mediaType := header.Header.Get("Content-Type")

	videoData, err := io.ReadAll(videoFile)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't read video file", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload this video", err)
		return
	}

	var extension string

	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
		return
	}

	switch mimeType {
	case "video/mp4":
		extension = ".mp4"
		break
	default:
		respondWithError(w, http.StatusBadRequest, "Couldn't recognize file type", err)
		return
	}

	videoName := make([]byte, 32)
	_, err = rand.Read(videoName)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't generate random name", err)
		return
	}

	videoPathName := base64.RawURLEncoding.EncodeToString(videoName)

	tempFile, err := os.CreateTemp("", videoPathName)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't create temp file", err)
		return
	}

	defer os.Remove(videoPathName)
	defer tempFile.Close()

	_, err = io.Copy(tempFile, bytes.NewReader(videoData))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
		return
	}

	ratio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get aspect ratio", err)
		return
	}

	var prefix string

	switch ratio {
	case "16:9":
		prefix = "landscape"
		break
	case "9:16":
		prefix = "portrait"
		break
	default:
		prefix = "other"
		break
	}

	tempFile.Seek(0, io.SeekStart)

	converted, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video", err)
		return
	}

	fileName := prefix + "/" + videoPathName + extension

	cFile, err := os.ReadFile(converted)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read converted file", err)
		return
	}

	cReader := bytes.NewReader(cFile)

	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        cReader,
		ContentType: &mimeType,
	})
	if err != nil {
		fmt.Printf("%s region, %s bucket", cfg.s3Region, cfg.s3Bucket)
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video", err)
		return
	}

	url := fmt.Sprintf("%s,%s", cfg.s3Bucket, fileName)

	video.VideoURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't update video", err)
		return
	}

	video, err = cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
		return
	}

	vid, err := cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't convert video to signed video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, vid)
}
