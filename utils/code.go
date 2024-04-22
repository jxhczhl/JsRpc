package utils

import "fmt"

func ConcatCode(code string) string {
	// 拼接页面元素的js
	return fmt.Sprintf("(function(){return %s;})()", code)
}
