package log

import (
	"go.uber.org/zap"
)

var LOG *zap.SugaredLogger

func init() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	sugar := logger.Sugar()
	sugar.Infof("Failed to fetch URL: %s", "test")
	LOG = sugar
}
