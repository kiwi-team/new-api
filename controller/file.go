package controller

import (
	"context"
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"

	"github.com/gin-gonic/gin"
	"google.golang.org/genai"
)

type UploadFileRequest struct {
	FileUri string `json:"file_uri"`
}

func UploadFile(ctx *gin.Context) {
	request := UploadFileRequest{}
	err := ctx.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(ctx, err)
		return
	}
	f, err := UploadFileToGoogleStorage(ctx, request.FileUri)
	if err != nil {
		common.ApiErrorMsg(ctx, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"file_url":  f.DownloadURI,
		"mime_type": f.MIMEType,
	})

}

func UploadFileToGoogleStorage(ctx context.Context, fileUri string) (*genai.File, error) {
	channels, err := model.GetAllVertexChannels()
	if err != nil {
		return nil, err
	}
	num := len(channels)
	if num == 0 {
		return nil, errors.New("no vertex channel found")
	}
	index := common.GetRandomInt(num)
	channel := channels[index]
	setting := channel.GetSetting()
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return nil, errors.New("no vertex channel key found")
	}
	index = common.GetRandomInt(len(keys))
	key := keys[index]
	f, err := gemini.UploadFileToGoogle(ctx, fileUri, setting.GoogleFileBucket, key)
	if err != nil {
		return nil, err
	}
	if f.DownloadURI == "" {
		return nil, errors.New("failed to get download uri")
	}
	return f, nil
}
