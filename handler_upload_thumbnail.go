package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	var maxMemory int64
	maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)

	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
	}

	image, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file", err)
	}

	mediaType := header.Header.Get("Content-Type")

	imageData, err := io.ReadAll(image)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't read image", err)
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload this video", err)
	}

	var extension string

	switch mediaType {
	case "image/jpeg":
		extension = ".jpg"
		break
	case "image/png":
		extension = ".png"
		break
	case "image/gif":
		extension = ".gif"
		break
	case "image/webp":
		extension = ".webp"
		break
	default:
		respondWithError(w, http.StatusBadRequest, "Couldn't recognize file type", err)
	}

	path := filepath.Join(cfg.assetsRoot, videoIDString+"."+extension)

	f, err := os.Create(path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
	}

	_, err = io.Copy(f, bytes.NewReader(imageData))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
	}

	url := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, videoID, extension)

	video.ThumbnailURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't update video", err)
	}

	video, err = cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find video", err)
	}

	respondWithJSON(w, http.StatusOK, video)
}
