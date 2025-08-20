package controller

import (
	"errors"
	"fmt"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/middleware"
	"one-api/model"
	"one-api/service"
	"one-api/setting"
	"one-api/types"
	"time"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		newAPIError = types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied)
		return
	}

	playgroundRequest := &dto.PlayGroundRequest{}
	err := common.UnmarshalBodyReusable(c, playgroundRequest)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	if playgroundRequest.Model == "" {
		newAPIError = types.NewError(errors.New("请选择模型"), types.ErrorCodeInvalidRequest)
		return
	}
	c.Set("original_model", playgroundRequest.Model)
	group := playgroundRequest.Group
	userGroup := c.GetString("group")

	if group == "" {
		group = userGroup
	} else {
		if !setting.GroupInUserUsableGroups(group) && group != userGroup {
			newAPIError = types.NewError(errors.New("无权访问该分组"), types.ErrorCodeAccessDenied)
			return
		}
		c.Set("group", group)
	}
	c.Set("token_name", "playground-"+group)
	tags := make([]string, 0)
	channel, finalGroup, err := model.CacheGetRandomSatisfiedChannel(c, group, playgroundRequest.Model, 0, tags)
	if err != nil {
		message := fmt.Sprintf("当前分组 %s 下对于模型 %s 无可用渠道", finalGroup, playgroundRequest.Model)
		//newAPIError = service.OpenAIErrorWrapperLocal(errors.New(message), "get_playground_channel_failed", http.StatusInternalServerError)
		newAPIError = &types.NewAPIError{
			ErrorType:  "get_playground_channel_failed",
			StatusCode: http.StatusInternalServerError,
		}
		service.OpenAIErrorWrapperLocal(errors.New(message), "get_playground_channel_failed", http.StatusInternalServerError)
		return
	}
	middleware.SetupContextForSelectedChannel(c, channel, playgroundRequest.Model)

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeQueryDataError)
		return
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", group),
		Group:  group,
	}
	_ = middleware.SetupContextForToken(c, tempToken)
	_, newAPIError = getChannel(c, group, playgroundRequest.Model, 0)
	if newAPIError != nil {
		return
	}
	//middleware.SetupContextForSelectedChannel(c, channel, playgroundRequest.Model)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())

	Relay(c)
}
