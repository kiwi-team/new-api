package speech

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// mergeNative preserves native fields (including future provider extensions),
// explicit zero/false and array replacement. Request identity and transport
// controls are checked separately before a patch reaches this function.
func mergeNative(base, patch map[string]any) {
	for key, value := range patch {
		object, nested := value.(map[string]any)
		current, exists := base[key].(map[string]any)
		if nested && exists {
			mergeNative(current, object)
		} else {
			base[key] = value
		}
	}
}

// Native objects are also used for websocket initialization messages. Bound
// recursion and reject unimplemented quantity controls rather than allowing an
// extension to bypass the gateway's request/billing limits.
func validateNativeObject(object map[string]any, depth int) error {
	if depth > 32 {
		return errors.New("speech metadata nesting exceeds 32 levels")
	}
	for key, value := range object {
		if slices.Contains([]string{"authorization", "api_key", "apikey", "xi_api_key", "single_use_token", "access_token", "access_key", "headers", "base_url", "endpoint", "max_tokens", "max_output_tokens", "max_completion_tokens", "max_new_tokens", "duration", "seconds", "n", "batch_size", "num_outputs"}, strings.ToLower(key)) {
			return fmt.Errorf("speech metadata cannot set gateway-controlled field: %s", key)
		}
		switch item := value.(type) {
		case map[string]any:
			if err := validateNativeObject(item, depth+1); err != nil {
				return err
			}
		case []any:
			for _, element := range item {
				if err := validateNativeObject(map[string]any{"[]": element}, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func nativeObject(parent map[string]any, key string) (map[string]any, error) {
	value, exists := parent[key]
	if !exists {
		object := map[string]any{}
		parent[key] = object
		return object, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("speech metadata %s must be an object", key)
	}
	return object, nil
}

func nativeConflict(key string) error {
	return fmt.Errorf("speech metadata %s conflicts with the top-level parameter", key)
}

func (r *Request) prepareNativeMetadata() error {
	r.Native = map[string]any{}
	if len(r.Metadata) > 0 && strings.TrimSpace(string(r.Metadata)) != "null" {
		if err := common.Unmarshal(r.Metadata, &r.Native); err != nil || r.Native == nil {
			return errors.New("metadata must be an object")
		}
	}
	if err := validateNativeObject(r.Native, 0); err != nil {
		return err
	}
	for key := range r.Native {
		if slices.Contains([]string{"model", "model_id", "resource_id", "text", "text_prompt", "ssml", "stream", "output_format", "event", "session_id", "voices", "inputs", "flush", "close_socket", "keep_alive", "user", "app"}, strings.ToLower(key)) {
			return fmt.Errorf("speech metadata cannot set gateway-controlled field: %s", key)
		}
	}
	var controls map[string]any
	var voice, rate, format, instruction string
	switch r.Model {
	case QwenPlus, QwenFlash, QwenNext:
		var err error
		controls, err = nativeObject(r.Native, "input")
		if err != nil {
			return err
		}
		for _, key := range []string{"text", "text_prompt", "ssml"} {
			if _, exists := controls[key]; exists {
				return fmt.Errorf("speech metadata cannot override input.%s", key)
			}
		}
		voice, rate, format, instruction = "voice", "rate", "format", "instruction"
	case MiniMaxHD, MiniMaxTurbo:
		var err error
		controls, err = nativeObject(r.Native, "voice_setting")
		if err != nil {
			return err
		}
		voice, rate = "voice_id", "speed"
	case SeedTTS, SeedICL:
		var err error
		controls, err = nativeObject(r.Native, "req_params")
		if err != nil {
			return err
		}
		for _, key := range []string{"text", "ssml"} {
			if _, exists := controls[key]; exists {
				return fmt.Errorf("speech metadata cannot override req_params.%s", key)
			}
		}
		if value, exists := controls["model"]; exists {
			if r.Model != SeedICL || value != "seed-tts-2.0-standard" {
				return errors.New("Doubao effect version must match the gateway's seed-icl-2.0 configuration")
			}
		}
		voice = "speaker"
	case ElevenV4, ElevenTurbo:
		query, err := nativeObject(r.Native, "query")
		if err != nil {
			return err
		}
		for key, value := range query {
			if strings.EqualFold(key, "model_id") || strings.EqualFold(key, "model") {
				return errors.New("ElevenLabs metadata query cannot override model_id")
			}
			if key == "output_format" {
				text, ok := value.(string)
				if !ok || !slices.Contains([]string{"mp3_22050_32", "mp3_24000_48", "mp3_44100_32", "mp3_44100_64", "mp3_44100_96", "mp3_44100_128", "mp3_44100_192"}, text) {
					return errors.New("ElevenLabs output_format must use a supported MP3 encoding")
				}
			}
			if _, object := value.(map[string]any); object {
				return fmt.Errorf("ElevenLabs query %s must be a scalar", key)
			}
			if _, array := value.([]any); array || value == nil {
				return fmt.Errorf("ElevenLabs query %s must be a scalar", key)
			}
		}
		if seed, exists := query["seed"]; exists {
			value, ok := seed.(float64)
			if !ok || value < 1 || value > 4294967295 || math.Trunc(value) != value {
				return errors.New("ElevenLabs seed must be an integer between 1 and 4294967295")
			}
		}
		if value, exists := r.Native["voice_settings"]; exists {
			settings, ok := value.(map[string]any)
			if !ok {
				return errors.New("ElevenLabs voice_settings must be an object")
			}
			for _, key := range []string{"stability", "clone_strength"} {
				if err := validateNativeNumber(settings, key, 0, 1, false); err != nil {
					return err
				}
			}
		}
		if value, exists := r.Native["pronunciation_dictionary_locators"]; exists {
			items, ok := value.([]any)
			if !ok {
				return errors.New("ElevenLabs pronunciation_dictionary_locators must be an array")
			}
			for _, item := range items {
				entry, ok := item.(map[string]any)
				if !ok {
					return errors.New("ElevenLabs dictionary locators must be objects")
				}
				for _, key := range []string{"pronunciation_dictionary_id", "version_id"} {
					text, ok := entry[key].(string)
					if !ok || strings.TrimSpace(text) == "" {
						return fmt.Errorf("ElevenLabs dictionary locator requires %s", key)
					}
				}
			}
		}
		return nil
	}
	// Preserve the flat extension syntax shipped with the initial integration.
	// New requests should use the provider's original nesting and units.
	legacy := map[string]string{"sample_rate": "sample_rate", "volume": "volume", "pitch": "pitch", "seed": "seed", "bit_rate": "bit_rate", "channels": "channels", "quality": "quality", "enable_cbr": "enable_cbr", "enable_aigc_tag": "enable_aigc_tag", "enable_ssml": "enable_ssml", "word_timestamp_enabled": "word_timestamp_enabled", "language_hints": "language_hints", "references": "references"}
	if r.Model == MiniMaxHD || r.Model == MiniMaxTurbo {
		legacy = map[string]string{"sample_rate": "sample_rate", "volume": "vol", "pitch": "pitch", "bit_rate": "bitrate", "channels": "channel"}
	} else if r.Model == SeedTTS || r.Model == SeedICL {
		legacy = map[string]string{"sample_rate": "sample_rate"}
	}
	for key, target := range legacy {
		value, exists := r.Native[key]
		if !exists {
			continue
		}
		destination := controls
		if r.Model == MiniMaxHD || r.Model == MiniMaxTurbo {
			switch key {
			case "volume":
				target = "vol"
			case "pitch":
			case "sample_rate", "channels", "bit_rate":
				var err error
				destination, err = nativeObject(r.Native, "audio_setting")
				if err != nil {
					return err
				}
				if key == "channels" {
					target = "channel"
				}
				if key == "bit_rate" {
					target = "bitrate"
					if number, ok := value.(float64); ok {
						value = number * 1000
					}
				}
			}
		} else if r.Model == SeedTTS || r.Model == SeedICL {
			var err error
			destination, err = nativeObject(controls, "audio_params")
			if err != nil {
				return err
			}
		}
		if previous, exists := destination[target]; exists && !reflect.DeepEqual(previous, value) {
			return nativeConflict(key)
		}
		destination[target] = value
		delete(r.Native, key)
	}
	if voice != "" {
		if value, exists := controls[voice]; exists {
			text, ok := value.(string)
			if !ok || (r.Voice != "" && r.Voice != text) {
				return nativeConflict(voice)
			}
			r.Voice = text
		}
	}
	if rate != "" {
		if value, exists := controls[rate]; exists {
			number, ok := value.(float64)
			if !ok || (r.Speed != nil && *r.Speed != number) {
				return nativeConflict(rate)
			}
			r.Speed = &number
		}
	}
	if instruction != "" {
		if value, exists := controls[instruction]; exists {
			text, ok := value.(string)
			if !ok || (r.Instructions != "" && r.Instructions != text) {
				return nativeConflict(instruction)
			}
			r.Instructions = text
		}
	}
	audio := controls
	if r.Model == MiniMaxHD || r.Model == MiniMaxTurbo {
		var err error
		audio, err = nativeObject(r.Native, "audio_setting")
		if err != nil {
			return err
		}
		format = "format"
		if options, exists := r.Native["stream_options"]; exists {
			object, ok := options.(map[string]any)
			_, controlled := object["exclude_aggregated_audio"]
			if !ok || controlled {
				return errors.New("MiniMax exclude_aggregated_audio is controlled by the gateway")
			}
		}
		if value, exists := r.Native["emotion"]; exists {
			if previous, exists := controls["emotion"]; exists && !reflect.DeepEqual(previous, value) {
				return nativeConflict("emotion")
			}
			controls["emotion"] = value
			delete(r.Native, "emotion")
		}
	} else if r.Model == SeedTTS || r.Model == SeedICL {
		var err error
		audio, err = nativeObject(controls, "audio_params")
		if err != nil {
			return err
		}
		format = "format"
		if value, exists := audio["speech_rate"]; exists {
			number, ok := value.(float64)
			if !ok || number < -50 || number > 100 || math.Trunc(number) != number {
				return errors.New("Doubao speech_rate must be an integer between -50 and 100")
			}
			if r.Speed != nil && math.Round((*r.Speed-1)*100) != number {
				return nativeConflict("speech_rate")
			}
			speed := 1 + number/100
			r.Speed = &speed
		}
		additions := map[string]any{}
		if value, exists := controls["additions"]; exists {
			if text, ok := value.(string); ok {
				if err := common.Unmarshal([]byte(text), &additions); err != nil || additions == nil {
					return errors.New("Doubao additions must encode a JSON object")
				}
			} else {
				var ok bool
				additions, ok = value.(map[string]any)
				if !ok {
					return errors.New("Doubao additions must be an object or a JSON object string")
				}
			}
			if err := validateNativeObject(additions, 0); err != nil {
				return err
			}
		}
		if value, exists := additions["context_texts"]; exists {
			items, ok := value.([]any)
			if !ok {
				return errors.New("Doubao context_texts must be a string array")
			}
			if r.Model == SeedICL && len(items) > 0 {
				return errors.New("seed-icl-2.0 does not support context_texts")
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return errors.New("Doubao context_texts must be a string array")
				}
			}
			if r.Instructions != "" && (len(items) != 1 || items[0] != r.Instructions) {
				return nativeConflict("context_texts")
			}
		}
		if r.Instructions != "" {
			additions["context_texts"] = []any{r.Instructions}
		}
		if len(additions) > 0 {
			controls["additions"] = additions
		}
	}
	if format != "" {
		if value, exists := audio[format]; exists {
			text, ok := value.(string)
			if text == "ogg_opus" && (r.Model == SeedTTS || r.Model == SeedICL) {
				text = "opus"
			}
			if !ok || text == "" || (r.ResponseFormat != "" && r.ResponseFormat != text) {
				return nativeConflict(format)
			}
			r.ResponseFormat = text
			if text == "opus" && (r.Model == SeedTTS || r.Model == SeedICL) {
				audio[format] = "ogg_opus"
			}
		}
	}
	// Decode only known controls for local validation; the original native map
	// remains authoritative for forwarding unknown vendor fields.
	known := map[string]any{}
	if r.Model == MiniMaxHD || r.Model == MiniMaxTurbo {
		known["volume"], known["pitch"] = controls["vol"], controls["pitch"]
		known["sample_rate"], known["channels"] = audio["sample_rate"], audio["channel"]
		if value, exists := audio["bitrate"]; exists {
			number, ok := value.(float64)
			if !ok || math.Trunc(number) != number {
				return errors.New("MiniMax bitrate must be an integer in bps")
			}
			known["bit_rate"] = number / 1000
		}
	} else if r.Model == SeedTTS || r.Model == SeedICL {
		known["sample_rate"] = audio["sample_rate"]
	} else {
		known = controls
	}
	data, err := common.Marshal(known)
	if err != nil {
		return err
	}
	if err := common.Unmarshal(data, &r.Extensions); err != nil {
		return fmt.Errorf("invalid native speech parameters: %w", err)
	}
	return nil
}

func validateNativeNumber(object map[string]any, key string, low, high float64, integer bool) error {
	value, exists := object[key]
	if !exists {
		return nil
	}
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < low || number > high || (integer && math.Trunc(number) != number) {
		return fmt.Errorf("native speech parameter %s must be between %g and %g", key, low, high)
	}
	return nil
}

func (r *Request) validateNativeSynthesis() error {
	switch r.Model {
	case MiniMaxHD, MiniMaxTurbo:
		voice := r.Native["voice_setting"].(map[string]any)
		if value, exists := voice["vol"]; exists && value == float64(0) {
			return errors.New("MiniMax vol must be greater than zero")
		}
		if value, exists := r.Native["timbre_weights"]; exists {
			items, ok := value.([]any)
			if !ok || len(items) == 0 || len(items) > 4 || r.Voice != "" {
				return errors.New("MiniMax timbre_weights requires one to four voices and an empty voice")
			}
			for _, item := range items {
				entry, ok := item.(map[string]any)
				if !ok {
					return errors.New("MiniMax timbre_weights entries must be objects")
				}
				voice, ok := entry["voice_id"].(string)
				if !ok || strings.TrimSpace(voice) == "" || entry["weight"] == nil {
					return errors.New("MiniMax timbre_weights requires voice_id and weight")
				}
				if err := validateNativeNumber(entry, "weight", 1, 100, true); err != nil {
					return err
				}
			}
		}
		if value, exists := r.Native["voice_modify"]; exists {
			object, ok := value.(map[string]any)
			if !ok {
				return errors.New("MiniMax voice_modify must be an object")
			}
			for _, key := range []string{"pitch", "intensity", "timbre"} {
				if err := validateNativeNumber(object, key, -100, 100, true); err != nil {
					return err
				}
			}
		}
	case SeedTTS, SeedICL:
		params := r.Native["req_params"].(map[string]any)
		audio := params["audio_params"].(map[string]any)
		if err := validateNativeNumber(audio, "loudness_rate", -50, 100, true); err != nil {
			return err
		}
		if err := validateNativeNumber(audio, "bit_rate", 16000, 160000, true); err != nil {
			return err
		}
		if _, exists := audio["bit_rate"]; exists && r.ResponseFormat == "pcm" {
			return errors.New("Doubao PCM does not support bit_rate")
		}
		if additions, ok := params["additions"].(map[string]any); ok {
			if object, exists := additions["post_process"]; exists {
				post, ok := object.(map[string]any)
				if !ok {
					return errors.New("Doubao post_process must be an object")
				}
				if err := validateNativeNumber(post, "pitch", -12, 12, true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Text replacement can expand the synthesized text. Keep the original request
// identity, but reserve against a bounded conservative expansion; actual billing
// still uses the upstream usage. No expansion is sent to the provider twice.
func (r *Request) estimatedSynthesisText(text string, limit int) (string, error) {
	var rules [][2]string
	var dictionary any
	if r.Model == QwenPlus || r.Model == QwenFlash || r.Model == QwenNext {
		input := r.Native["input"].(map[string]any)
		if value, exists := input["hot_fix"]; exists {
			fix, ok := value.(map[string]any)
			if !ok {
				return "", errors.New("Qwen hot_fix must be an object")
			}
			if value, exists := fix["replace"]; exists {
				items, ok := value.([]any)
				if !ok || len(items) > 5000 {
					return "", errors.New("Qwen hot_fix.replace must contain at most 5000 replacement objects")
				}
				for _, item := range items {
					object, ok := item.(map[string]any)
					if !ok || len(object) != 1 {
						return "", errors.New("Qwen replacement entries must contain exactly one text pair")
					}
					for from, value := range object {
						to, ok := value.(string)
						if !ok || from == "" {
							return "", errors.New("Qwen replacements require a nonempty source and a string value")
						}
						rules = append(rules, [2]string{from, to})
					}
				}
			}
		}
	} else if r.Model == MiniMaxHD || r.Model == MiniMaxTurbo {
		dictionary = r.Native["pronunciation_dict"]
	} else if params, ok := r.Native["req_params"].(map[string]any); ok {
		if additions, ok := params["additions"].(map[string]any); ok {
			dictionary = additions["pronunciation_dict"]
		}
	}
	if dictionary != nil {
		object, ok := dictionary.(map[string]any)
		if !ok {
			return "", errors.New("pronunciation_dict must be an object")
		}
		if value, exists := object["tone"]; exists {
			items, ok := value.([]any)
			if !ok || len(items) > 5000 {
				return "", errors.New("pronunciation_dict.tone must contain at most 5000 rules")
			}
			seen := map[string]bool{}
			for _, item := range items {
				entry, ok := item.(string)
				if !ok {
					return "", errors.New("pronunciation rules must be strings")
				}
				from, to, found := strings.Cut(entry, "/")
				if !found || from == "" || seen[from] {
					return "", errors.New("pronunciation rules require unique nonempty source text and a slash")
				}
				if (r.Model == SeedTTS || r.Model == SeedICL) && (utf8.RuneCountInString(from) > 9 || strings.ContainsFunc(from, unicode.IsSpace)) {
					return "", errors.New("Doubao pronunciation sources must be at most 9 characters without whitespace")
				}
				seen[from] = true
				rules = append(rules, [2]string{from, to})
			}
		}
	}
	for _, rule := range rules {
		occurrences := strings.Count(text, rule[0])
		size := int64(utf8.RuneCountInString(text)) + int64(occurrences)*int64(utf8.RuneCountInString(rule[1])-utf8.RuneCountInString(rule[0]))
		if size > int64(limit) {
			return "", errors.New("native text replacement exceeds the speech input limit")
		}
		text = strings.ReplaceAll(text, rule[0], rule[1])
	}
	return text, nil
}
