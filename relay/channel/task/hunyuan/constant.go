// https://cloud.tencent.com/document/product/1668/124632#SDK
package hunyuan

type HunyuanTaskSubmitRequest struct {
	Prompt     string `json:"Prompt"`
	Resolution string `json:"Resolution,omitempty"`
	Seed       int    `json:"Seed,omitempty"`
	Image      Image  `json:"Image,omitempty"`
	Revise     int    `json:"Revise,omitempty"`
}

type Image struct {
	Url    string `json:"Url,omitempty"`
	Base64 string `json:"Base64,omitempty"`
}

/*
	{
	    "Response": {
	        "JobId": "1344213737283272704",
	        "RequestId": "af61e1d4-0931-4dc5-b5a9-eb89bae54616"
	    }
	}
*/
type HunyuanTaskSubmitResponse struct {
	Response struct {
		JobId     string `json:"JobId,omitempty"`
		RequestId string `json:"RequestId,omitempty"`
	} `json:"Response,omitempty"`
}

type HunyuanTaskResultRequest struct {
	Action  string `json:"Action"`
	Version string `json:"Version"`
	Region  string `json:"Region"`
	JobId   string `json:"JobId"`
}

// 1：等待中、2：运行中、4：处理失败、5：处理完成。
const (
	HunyuanTaskStatusWaiting = "1"
	HunyuanTaskStatusRunning = "2"
	HunyuanTaskStatusFailed  = "4"
	HunyuanTaskStatusSuccess = "5"
)

const (
	HunyuanRegionShanghai  = "ap-shanghai"
	HunyuanRegionGuangzhou = "ap-guangzhou"
)

/*
	{
	    "Response": {
	        "JobErrorCode": "",
	        "JobErrorMsg": "",
	        "JobStatusCode": "5",
	        "JobStatusMsg": "处理完成",
	        "RequestId": "e4a4eef5-f4aa-40ff-aae0-5e51c5ef5b1e",
	        "ResultDetails": [
	            "Success"
	        ],
	        "ResultImage": [
	            "https://cos.ap-guangzhou.myqcloud.com/xxx.jpg"
	        ],
	        "RevisedPrompt": [
	            "一只可爱的小狗正蹲在草地上，它的小耳朵竖立着，鼻子微微皱起，显得十分警觉，小狗的毛色是鲜亮的红色"
	        ]
	    }
	}
*/
type HunyuanTaskResultResponse struct {
	Response struct {
		JobErrorCode  string   `json:"JobErrorCode,omitempty"`
		JobErrorMsg   string   `json:"JobErrorMsg,omitempty"`
		JobStatusCode string   `json:"JobStatusCode,omitempty"`
		JobStatusMsg  string   `json:"JobStatusMsg,omitempty"`
		RequestId     string   `json:"RequestId,omitempty"`
		ResultDetails []string `json:"ResultDetails,omitempty"`
		ResultImage   []string `json:"ResultImage,omitempty"`
		RevisedPrompt []string `json:"RevisedPrompt,omitempty"`
	} `json:"Response,omitempty"`
}
