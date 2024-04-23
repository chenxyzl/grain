package singal

import (
	"github.com/chenxyzl/grain/utils/share"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func callFuncSlice(fs []func()) {
	defer share.Recover()
	for _, f := range fs {
		if f != nil {
			f()
		}
	}
}

func WaitStopSignal(logger *slog.Logger, beforeFunc ...func()) {
	// signal.Notify的ch信道是阻塞的(signal.Notify不会阻塞发送信号), 需要设置缓冲
	signals := make(chan os.Signal, 1)
	// It is not possible to block SIGKILL or syscall.SIGSTOP
	signal.Notify(signals, os.Interrupt, os.Kill, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
Quit:
	for {
		sig := <-signals
		logger.Warn("get signal", "signal", sig.String())
		//
		switch sig {
		case syscall.SIGHUP:
			logger.Warn("get signal do nothings")
		default:
			logger.Warn("get signal will quit")
			break Quit
		}
	}
	//
	callFuncSlice(beforeFunc)
	//
	logger.Warn("app exit success")
}
