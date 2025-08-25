package lockstep

import (
	"log"
)

// Log 提供简单的日志接口，默认输出到控制台，可自定义
var Log = struct {
	Debug   func(format string, v ...interface{})
	Info    func(format string, v ...interface{})
	Warning func(format string, v ...interface{})
	Error   func(format string, v ...interface{})
}{
	Debug: func(format string, v ...interface{}) {
		log.Printf("[DEBUG] "+format+"\n", v...)
	},
	Info: func(format string, v ...interface{}) {
		log.Printf("[INFO] "+format+"\n", v...)
	},
	Warning: func(format string, v ...interface{}) {
		log.Printf("[WARN] "+format+"\n", v...)
	},
	Error: func(format string, v ...interface{}) {
		log.Printf("[ERROR] "+format, v...)
	},
}
