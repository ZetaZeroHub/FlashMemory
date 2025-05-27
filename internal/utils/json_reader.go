package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

// ReadJSONFile 读取 JSON 文件并将其解析为 map
func ReadJSONFile(filePath string) (map[string]interface{}, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// 读取文件内容
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// 定义一个 map 来存储解析后的数据
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	// 返回解析后的 map
	return result, nil
}

// ReadJSONArrayFile 读取 JSON 文件并将其解析为一个数组（slice）
func ReadJSONArrayFile(filePath string) ([]interface{}, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// 读取文件内容
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// 定义一个切片用于存储解析后的数据
	var result []interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON array: %v", err)
	}

	// 返回解析后的切片
	return result, nil
}

func PrettyJSON(v interface{}) (string, error) {
	// 第二个参数 prefix 通常设为空串；第三个参数 indent 表示缩进字符串
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
