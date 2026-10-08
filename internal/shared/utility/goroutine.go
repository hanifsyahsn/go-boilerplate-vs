package utility

import "github.com/bytedance/gopkg/util/logger"

func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("panic recovered: %v", r)
			}
		}()
		fn()
	}()
}
