package elevenlabs

var ModelList = []string{
	"eleven_monolingual_v1",
	"eleven_multilingual_v2",
	"eleven_multilingual_v3",
}

var VoideNameIDMap = map[string]string{
	"Rachel":    "21m00Tcm4TlvDq8ikWAM",
	"Drew":      "29vD33N1CtxCmqQRPOHJ",
	"Clyde":     "2EiwWnXFnvU5JabPnv8n",
	"Paul":      "5Q0t7uMcjvnagumLfvZi",
	"Aria":      "9BWtsMINqrJLrRacOk9x",
	"Domi":      "AZnzlk1XvdvUeBnXmlld",
	"Dave":      "CYw3kZ02Hs0563khs1Fj",
	"Roger":     "CwhRBWXzGAHq8TQ4Fs17",
	"Fin":       "D38z5RcWu1voky8WS1ja",
	"Sarah":     "EXAVITQu4vr4xnSDxMaL",
	"Antoni":    "ErXwobaYiN019PkySvjV",
	"Laura":     "FGY2WhTYpPnrIDTdsKH5",
	"Thomas":    "GBv7mTt0atIp3Br8iCZE",
	"Charlie":   "IKne3meq5aSn9XLyUdCD",
	"George":    "JBFqnCBsd6RMkjVDRZzb",
	"Emily":     "LcfcDJNUP1GQjkzn1xUU",
	"Elli":      "MF3mGyEYCl7XYWbV9V6O",
	"Callum":    "N2lVS1w4EtoT3dr4eOWO",
	"Patrick":   "ODq5zmih8GrVes37Dizd",
	"River":     "SAz9YHcvj6GT2YYXdXww",
	"Harry":     "SOYHLrjzK2X1ezoPC6cr",
	"Liam":      "TX3LPaxmHKxFdv7VOQHJ",
	"Dorothy":   "ThT5KcBeYPX3keUQqHPh",
	"Josh":      "TxGEqnHWrfWFTfGW9XjX",
	"Arnold":    "VR6AewLTigWG4xSOukaG",
	"Charlotte": "XB0fDUnXU5powFXDhCwa",
	"Alice":     "Xb7hH8MSUJpSbSDYk0k2",
	"Matilda":   "XrExE9yKIg1WjnnlVkGX",
	"James":     "ZQe5CZNOzWyzPSCn5a3c",
	"Joseph":    "Zlb1dXrM653N07WRdFW3",
	"Will":      "bIHbv24MWmeRgasZH58o",
	"Jeremy":    "bVMeCyTHy58xNoL34h3p",
	"Jessica":   "cgSgspJ2msm6clMCkdW9",
	"Eric":      "cjVigY5qzO86Huf0OWal",
	"Michael":   "flq6f7yk4E4fJM5XTYuZ",
	"Ethan":     "g5CIjZEefAph4nQFvHAz",
	"Chris":     "iP95p4xoKVk53GoZ742B",
	"Gigi":      "jBpfuIE2acCO8z3wKNLl",
	"Freya":     "jsCqWAovK2LkecY7zXl4",
	"Brian":     "nPczCjzI2devNBz1zQrb",
	"Grace":     "oWAxZDx7w5VEj9dCyTzz",
	"Daniel":    "onwK4e9ZLuTAKqWW03F9",
	"Lily":      "pFZP5JQG7iQjIQuC4Bku",
	"Serena":    "pMsXgVXv3BLzUgSXRplE",
	"Adam":      "pNInz6obpgDQGcFmaJgB",
	"Nicole":    "piTKgcLEGmPE4e6mEKli",
	"Bill":      "pqHfZKP75CvOlQylNhV4",
	"Jessie":    "t0jbNlBVZ17f02VDIeMI",
	"Sam":       "yoZ06aMxZJJ28mfd3POQ",
	"Glinda":    "z9fAnlkpzviPz146aGWa",
	"Giovanni":  "zcAOhNBS3c14rBihAFp1",
	"Mimi":      "zrHiDhphv9ZnVXBqCLjz",
}

type TTSRequest struct {
	ModelID       string         `json:"model_id"`
	Text          string         `json:"text"`
	VoiceSettings *VoiceSettings `json:"voice_settings,omitempty"`
}

type VoiceSettings struct {
	Stability       float64 `json:"stability"`
	UseSpeakerBoost bool    `json:"use_speaker_boost"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Style           float64 `json:"style"`
	Speed           float64 `json:"speed"`
}
