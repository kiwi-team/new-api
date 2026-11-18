package common

import (
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const Seedream55Model = "doubao-seedream-5-5-261028"

var seedreamImageDataURL = regexp.MustCompile(`^data:image/[a-z0-9.+-]+;base64,[A-Za-z0-9+/]+={0,2}$`)

// PrepareSeedream55Request translates the shared image contract into Ark JSON
// without consulting the task plugin registry. Both pricing and the native
// adaptor use this validation, including metadata and uploaded references.
func PrepareSeedream55Request(c *gin.Context, request dto.ImageRequest) (map[string]any, map[string]any, error) {
	count, err := request.ImageCount(false)
	if err != nil || count != 1 {
		return nil, nil, fmt.Errorf("n must be 1 for this model")
	}
	if request.Stream != nil && *request.Stream {
		return nil, nil, fmt.Errorf("stream is not supported by this model")
	}
	if len(request.Mask) > 0 {
		return nil, nil, fmt.Errorf("mask is not supported")
	}
	encoded, err := common.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	var source map[string]any
	if err := common.Unmarshal(encoded, &source); err != nil {
		return nil, nil, err
	}
	for key, raw := range request.Extra {
		if _, exists := source[key]; !exists {
			var value any
			if err := common.Unmarshal(raw, &value); err != nil {
				return nil, nil, err
			}
			source[key] = value
		}
	}
	if c != nil && c.Request.MultipartForm != nil {
		for key, values := range c.Request.MultipartForm.Value {
			if len(values) != 1 {
				return nil, nil, fmt.Errorf("%s must be provided once", key)
			}
			var value any = values[0]
			switch key {
			case "metadata", "images", "layer_decomposition_options", "optimize_prompt_options", "tools":
				if err := common.UnmarshalJsonStr(values[0], &value); err != nil {
					return nil, nil, fmt.Errorf("%s must be JSON", key)
				}
			case "seed", "guidance_scale":
				value, err = strconv.ParseFloat(values[0], 64)
			case "watermark", "optimize_prompt", "layer_decomposition":
				if values[0] != "true" && values[0] != "false" {
					return nil, nil, fmt.Errorf("%s must be a boolean", key)
				}
				value = values[0] == "true"
			}
			if err != nil {
				return nil, nil, fmt.Errorf("invalid %s", key)
			}
			// The request model is pinned independently of form/metadata options.
			if key != "model" && key != "n" && key != "stream" {
				source[key] = value
			}
		}
	}
	metadata, ok := source["metadata"].(map[string]any)
	if source["metadata"] != nil && !ok {
		return nil, nil, fmt.Errorf("metadata must be an object")
	}
	body := map[string]any{"model": request.Model}
	for _, key := range []string{"seed", "guidance_scale", "watermark", "optimize_prompt", "optimize_prompt_options", "layer_decomposition", "layer_decomposition_options", "safety_identifier", "tools", "sequential_image_generation", "sequential_image_generation_options"} {
		if value, exists := metadata[key]; exists {
			body[key] = value
		}
		if value, exists := source[key]; exists {
			body[key] = value
		}
	}
	for _, key := range []string{"prompt", "size", "background", "output_format", "response_format"} {
		if value, exists := source[key]; exists {
			body[key] = value
		}
	}
	for _, key := range []string{"watermark", "optimize_prompt", "layer_decomposition"} {
		if value, exists := body[key]; exists {
			if _, ok := value.(bool); !ok {
				return nil, nil, fmt.Errorf("%s must be a boolean", key)
			}
		}
	}
	layered := body["layer_decomposition"] == true
	if !layered && strings.TrimSpace(request.Prompt) == "" {
		return nil, nil, fmt.Errorf("prompt is required")
	}
	if _, exists := body["sequential_image_generation"]; exists {
		return nil, nil, fmt.Errorf("sequential_image_generation is not supported by this model")
	}
	sequentialOptions, ok := body["sequential_image_generation_options"].(map[string]any)
	if body["sequential_image_generation_options"] != nil && !ok {
		return nil, nil, fmt.Errorf("sequential_image_generation_options must be an object")
	}
	if value, exists := sequentialOptions["max_images"]; exists {
		quantity, ok := value.(float64)
		if !ok || quantity < 1 || quantity > 15 || quantity != math.Trunc(quantity) {
			return nil, nil, fmt.Errorf("sequential_image_generation_options.max_images must be an integer between 1 and 15")
		}
	}
	if _, exists := body["tools"]; exists {
		return nil, nil, fmt.Errorf("tools are not supported by this model")
	}
	if value, exists := body["seed"]; exists {
		seed, ok := value.(float64)
		if !ok || seed != math.Trunc(seed) {
			return nil, nil, fmt.Errorf("seed must be an integer")
		}
	}
	if _, exists := body["guidance_scale"]; exists {
		if _, ok := body["guidance_scale"].(float64); !ok {
			return nil, nil, fmt.Errorf("guidance_scale must be a number")
		}
	}
	images := []any{}
	for _, key := range []string{"image", "images"} {
		if value, exists := source[key]; exists {
			if items, ok := value.([]any); ok {
				images = append(images, items...)
			} else {
				images = append(images, value)
			}
		}
	}
	if c != nil && c.Request.MultipartForm != nil {
		for field, files := range c.Request.MultipartForm.File {
			if field != "image" && !(strings.HasPrefix(field, "image[") && strings.HasSuffix(field, "]")) {
				return nil, nil, fmt.Errorf("%s is not supported", field)
			}
			for _, file := range files {
				if len(images) >= 10 || file.Size > 30<<20 {
					return nil, nil, fmt.Errorf("at most 10 reference images of up to 30 MB are supported")
				}
				reader, err := file.Open()
				if err != nil {
					return nil, nil, err
				}
				data, readErr := io.ReadAll(io.LimitReader(reader, (30<<20)+1))
				reader.Close()
				if readErr != nil || len(data) > 30<<20 {
					return nil, nil, fmt.Errorf("unable to read image within the 30 MB limit")
				}
				mime := strings.ToLower(file.Header.Get("Content-Type"))
				if !strings.HasPrefix(mime, "image/") {
					mime = http.DetectContentType(data)
				}
				if !strings.HasPrefix(mime, "image/") {
					mime = "image/png"
				}
				images = append(images, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
			}
		}
	}
	if len(images) > 10 || layered && len(images) != 1 {
		return nil, nil, fmt.Errorf("at most 10 reference images are supported; layer decomposition requires exactly one input image")
	}
	for _, image := range images {
		value, ok := image.(string)
		if !ok || !(strings.HasPrefix(strings.ToLower(value), "https://") || strings.HasPrefix(strings.ToLower(value), "http://") || seedreamImageDataURL.MatchString(value)) {
			return nil, nil, fmt.Errorf("image must be an HTTP URL or a data:image/<format>;base64 URL")
		}
	}
	if len(images) > 0 {
		body["image"] = images
	}
	outputs := 1
	if layered {
		outputs = 30
	}
	options, ok := body["layer_decomposition_options"].(map[string]any)
	if body["layer_decomposition_options"] != nil && !ok {
		return nil, nil, fmt.Errorf("layer_decomposition_options must be an object")
	}
	if value, exists := options["max_images"]; exists {
		quantity, ok := value.(float64)
		if !ok || quantity < 2 || quantity > 100 || quantity != float64(int(quantity)) {
			return nil, nil, fmt.Errorf("layer_decomposition_options.max_images must be an integer between 2 and 100")
		}
		if layered {
			outputs = int(quantity)
		}
	}
	if value, exists := options["text_recognition"]; exists {
		if _, ok := value.(bool); !ok {
			return nil, nil, fmt.Errorf("layer_decomposition_options.text_recognition must be a boolean")
		}
	}
	promptOptions, ok := body["optimize_prompt_options"].(map[string]any)
	if body["optimize_prompt_options"] != nil && !ok {
		return nil, nil, fmt.Errorf("optimize_prompt_options must be an object")
	}
	if promptOptions["mode"] != nil && promptOptions["mode"] != "standard" {
		return nil, nil, fmt.Errorf("optimize_prompt_options.mode must be standard")
	}
	if format, exists := body["output_format"]; exists && format != "png" && format != "jpeg" {
		return nil, nil, fmt.Errorf("output_format must be png or jpeg")
	}
	if format, exists := body["response_format"]; exists && format != "" && format != "url" && format != "b64_json" {
		return nil, nil, fmt.Errorf("response_format must be url or b64_json")
	}
	if background, exists := body["background"]; exists {
		if layered || background != "auto" && background != "opaque" && background != "transparent" {
			return nil, nil, fmt.Errorf("background must be auto, opaque or transparent and cannot be used with layer decomposition")
		}
		if background == "transparent" && body["output_format"] == "jpeg" {
			return nil, nil, fmt.Errorf("background transparent cannot be combined with output_format jpeg")
		}
	}
	pixels := int64(4194304)
	size := strings.ToUpper(strings.TrimSpace(request.Size))
	switch size {
	case "1K":
		pixels = 1048576
	case "1.5K":
		pixels = 2359296
	case "2K":
	case "", "AUTO":
		delete(body, "size")
		if layered {
			body["size"] = "auto"
		}
	default:
		width, height, valid := seedream55Dimensions(size)
		if layered || !valid || width*height < 921600 || width*height > 4624220 || width > 16*height || height > 16*width {
			return nil, nil, fmt.Errorf("size is outside the model's pixel and aspect-ratio limits")
		}
		pixels = width * height
		size = fmt.Sprintf("%dx%d", width, height)
	}
	if size != "" && size != "AUTO" {
		body["size"] = size
	}
	facts := map[string]any{"images_up_to_1_5k": float64(0), "images_above_1_5k": float64(0), "input_images": float64(len(images)), "layer_decomposition": layered}
	if pixels <= 2610000 {
		facts["images_up_to_1_5k"] = float64(outputs)
	} else {
		facts["images_above_1_5k"] = float64(outputs)
	}
	return body, facts, nil
}

func seedream55Dimensions(size string) (int64, int64, bool) {
	size = strings.NewReplacer("X", "x", "×", "x", "*", "x").Replace(size)
	w, h, ok := strings.Cut(size, "x")
	width, wErr := strconv.ParseInt(strings.TrimSpace(w), 10, 64)
	height, hErr := strconv.ParseInt(strings.TrimSpace(h), 10, 64)
	return width, height, ok && wErr == nil && hErr == nil && width > 0 && height > 0 && width <= 4624220 && height <= 4624220
}

// Seedream55ResponseUsage returns only measured, bounded facts. Missing sizes
// or invalid vendor quantities leave the corresponding reservation unchanged.
func Seedream55ResponseUsage(body map[string]any) (map[string]any, int) {
	entries, _ := body["data"].([]any)
	if entry, ok := body["data"].(map[string]any); ok {
		entries = []any{entry}
	}
	urls, base64s := []map[string]any{}, []map[string]any{}
	for _, value := range entries {
		entry, _ := value.(map[string]any)
		if url, _ := entry["url"].(string); strings.TrimSpace(url) != "" {
			urls = append(urls, entry)
		}
		if data, _ := entry["b64_json"].(string); strings.TrimSpace(data) != "" {
			base64s = append(base64s, entry)
		}
	}
	payloads := urls
	if len(base64s) > len(urls) {
		payloads = base64s
	}
	count := len(payloads)
	if count == 0 || count > 100 {
		return nil, 0
	}
	usage, _ := body["usage"].(map[string]any)
	if value, exists := usage["generated_images"]; exists && value != float64(count) {
		return nil, 0
	}
	facts := map[string]any{}
	if quantity, ok := usage["input_images"].(float64); ok && quantity >= 0 && quantity <= 10 && quantity == float64(int(quantity)) {
		facts["input_images"] = quantity
	}
	lower, higher := 0, 0
	for _, entry := range payloads {
		size, _ := entry["size"].(string)
		width, height, valid := seedream55Dimensions(size)
		if !valid {
			return facts, count
		}
		if width*height <= 2610000 {
			lower++
		} else {
			higher++
		}
	}
	maps.Copy(facts, map[string]any{"images_up_to_1_5k": float64(lower), "images_above_1_5k": float64(higher)})
	return facts, count
}
