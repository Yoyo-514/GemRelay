package config

import (
	"fmt"
	"os"
	"time"
)

func Logf(format string, args ...interface{}) {
	if Runtime().LogRequests {
		fmt.Fprintf(os.Stderr, "[%s] %s\n",
			time.Now().Format("15:04:05"),
			fmt.Sprintf(format, args...))
	}
}
