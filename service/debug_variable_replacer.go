package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/tidwall/gjson"
)

// VariableReplacer 变量替换器
type VariableReplacer struct {
	variables    map[string]interface{}
	prevResponse map[string]interface{}
}

// NewVariableReplacer 创建变量替换器
func NewVariableReplacer(variables map[string]interface{}, prevResponse map[string]interface{}) *VariableReplacer {
	return &VariableReplacer{
		variables:    variables,
		prevResponse: prevResponse,
	}
}

// Replace 替换模板中的所有变量
func (r *VariableReplacer) Replace(template string) (string, error) {
	// 先替换 prev_response 变量
	result, err := r.replacePrevResponse(template)
	if err != nil {
		return "", err
	}

	// 再替换普通变量
	result, err = r.replaceSimple(result)
	if err != nil {
		return "", err
	}

	return result, nil
}

// replaceSimple 替换简单变量 {{variable_name}}
func (r *VariableReplacer) replaceSimple(template string) (string, error) {
	// 匹配 {{variable_name}} 格式
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)

	result := re.ReplaceAllStringFunc(template, func(match string) string {
		// 提取变量名
		varName := strings.TrimSpace(match[2 : len(match)-2])

		// 跳过 prev_response 变量（已经处理过）
		if strings.HasPrefix(varName, "prev_response.") {
			return match
		}

		// 获取变量值
		value, exists := r.variables[varName]
		if !exists {
			return match // 保持原样
		}

		// 转换为字符串
		return r.valueToString(value)
	})

	return result, nil
}

// replacePrevResponse 替换 prev_response 变量 {{prev_response.path.to.value}}
func (r *VariableReplacer) replacePrevResponse(template string) (string, error) {
	if r.prevResponse == nil || len(r.prevResponse) == 0 {
		return template, nil
	}

	// 匹配 {{prev_response.xxx}} 格式
	re := regexp.MustCompile(`\{\{prev_response\.([^}]+)\}\}`)

	// 将 prevResponse 转换为 JSON 字符串，以便使用 gjson
	jsonData, err := common.Marshal(r.prevResponse)
	if err != nil {
		return "", fmt.Errorf("failed to marshal prev response: %w", err)
	}

	result := re.ReplaceAllStringFunc(template, func(match string) string {
		// 提取 JSONPath，去掉 {{prev_response. 和 }}
		jsonPath := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{prev_response."), "}}"))

		// 使用 gjson 获取值
		value := gjson.GetBytes(jsonData, jsonPath)
		if !value.Exists() {
			return match // 保持原样
		}

		// 根据值类型返回
		return r.gjsonValueToString(value)
	})

	return result, nil
}

// valueToString 将值转换为字符串（用于 JSON 模板）
func (r *VariableReplacer) valueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		// 字符串需要加引号
		return fmt.Sprintf(`"%s"`, v)
	case bool:
		// 布尔值直接转换
		return strconv.FormatBool(v)
	case int, int8, int16, int32, int64:
		// 整数直接转换
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		// 浮点数直接转换
		return fmt.Sprintf("%v", v)
	case nil:
		return "null"
	default:
		// 对象和数组转换为 JSON
		data, err := common.Marshal(v)
		if err != nil {
			return fmt.Sprintf(`"%v"`, v)
		}
		return string(data)
	}
}

// gjsonValueToString 将 gjson.Result 转换为字符串
func (r *VariableReplacer) gjsonValueToString(value gjson.Result) string {
	switch value.Type {
	case gjson.String:
		return fmt.Sprintf(`"%s"`, value.String())
	case gjson.Number:
		return value.Raw
	case gjson.True, gjson.False:
		return value.Raw
	case gjson.Null:
		return "null"
	case gjson.JSON:
		return value.Raw
	default:
		return value.Raw
	}
}

// ReplaceInPath 替换路径中的变量 /v1/models/{model_name}
func (r *VariableReplacer) ReplaceInPath(path string) (string, error) {
	// 匹配 {variable_name} 格式
	re := regexp.MustCompile(`\{([^}]+)\}`)

	result := re.ReplaceAllStringFunc(path, func(match string) string {
		// 提取变量名
		varName := strings.TrimSpace(match[1 : len(match)-1])

		// 获取变量值
		value, exists := r.variables[varName]
		if !exists {
			return match // 保持原样
		}

		// 转换为字符串（路径中不需要引号）
		return r.pathValueToString(value)
	})

	return result, nil
}

// pathValueToString 将值转换为路径字符串（不加引号）
func (r *VariableReplacer) pathValueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ValidateJSON 验证替换后的 JSON 是否有效
func (r *VariableReplacer) ValidateJSON(jsonStr string) error {
	var js interface{}
	return common.Unmarshal([]byte(jsonStr), &js)
}

// ExtractVariables 从模板中提取所有变量名
func ExtractVariables(template string) []string {
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	matches := re.FindAllStringSubmatch(template, -1)

	variables := make([]string, 0)
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			varName := strings.TrimSpace(match[1])
			// 跳过 prev_response 变量
			if strings.HasPrefix(varName, "prev_response.") {
				continue
			}
			if !seen[varName] {
				variables = append(variables, varName)
				seen[varName] = true
			}
		}
	}

	return variables
}

// ExtractPathVariables 从路径中提取变量名
func ExtractPathVariables(path string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)

	variables := make([]string, 0)
	for _, match := range matches {
		if len(match) > 1 {
			variables = append(variables, strings.TrimSpace(match[1]))
		}
	}

	return variables
}

// ConvertType 类型转换
func ConvertType(value interface{}, targetType string) (interface{}, error) {
	switch targetType {
	case "string":
		return fmt.Sprintf("%v", value), nil

	case "number":
		switch v := value.(type) {
		case string:
			// 尝试转换为浮点数
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("cannot convert '%s' to number", v)
			}
			return f, nil
		case float64, float32:
			return v, nil
		case int, int8, int16, int32, int64:
			return float64(v.(int64)), nil
		case uint, uint8, uint16, uint32, uint64:
			return float64(v.(uint64)), nil
		default:
			return nil, fmt.Errorf("cannot convert %T to number", v)
		}

	case "boolean":
		switch v := value.(type) {
		case bool:
			return v, nil
		case string:
			return strconv.ParseBool(v)
		case int, int8, int16, int32, int64:
			return v != 0, nil
		default:
			return nil, fmt.Errorf("cannot convert %T to boolean", v)
		}

	case "object":
		switch v := value.(type) {
		case map[string]interface{}:
			return v, nil
		case string:
			var obj map[string]interface{}
			err := common.Unmarshal([]byte(v), &obj)
			return obj, err
		default:
			return nil, fmt.Errorf("cannot convert %T to object", v)
		}

	case "array":
		switch v := value.(type) {
		case []interface{}:
			return v, nil
		case string:
			var arr []interface{}
			err := common.Unmarshal([]byte(v), &arr)
			return arr, err
		default:
			return nil, fmt.Errorf("cannot convert %T to array", v)
		}

	default:
		return value, nil
	}
}
