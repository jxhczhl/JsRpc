package utils

import (
	"fmt"
	"os"
)

func IsExists(root string) bool {
	// 判断文件或者文件夹是否存在
	_, err := os.Stat(root)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func PrintJsRpc() {
	JsRpc := "       __       _______..______      .______     ______ \n      |  |     /       ||   _  \\     |   _  \\   /      |\n      |  |    |   (----`|  |_)  |    |  |_)  | |  ,----'\n.--.  |  |     \\   \\    |      /     |   ___/  |  |     \n|  `--'  | .----)   |   |  |\\  \\----.|  |      |  `----.\n \\______/  |_______/    | _| `._____|| _|       \\______|\n                                                        \n"
	fmt.Print(JsRpc)
}
