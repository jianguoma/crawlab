package services

import (
	"crawlab/constants"
	"crawlab/model"
	"crawlab/utils"
	"os/exec"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/apex/log"
	"github.com/spf13/viper"
)

func FinishOrCancelTask(ch chan string, cmd *exec.Cmd, t model.Task) {
	// 传入信号，此处阻塞
	signal := <-ch
	log.Infof("process received signal: %s", signal)

	if signal == constants.TaskCancel && cmd.Process != nil {

		err := cmd.Process.Kill()

		// 取消进程
		if err != nil {
			log.Errorf("process kill error: %s", err.Error())
			debug.PrintStack()

			t.Error = "kill process error: " + err.Error()
			t.Status = constants.StatusError
		} else {
			t.Error = "user kill the process ..."
			t.Status = constants.StatusCancelled
		}
	} else {
		// 保存任务
		t.Status = constants.StatusFinished
	}

	t.FinishTs = time.Now()
	_ = t.Save()
}

// 执行shell命令
func ExecuteShellCmd(cmdStr string, cwd string, t model.Task, s model.Spider) (err error) {
	log.Infof("cwd: %s", cwd)
	log.Infof("cmd: %s", cmdStr)

	// 生成执行命令
	var cmd *exec.Cmd
	if runtime.GOOS == constants.Windows {
		cmd = exec.Command("cmd", "/C", cmdStr)
	} else {
		cmd = exec.Command("sh", "-c", cmdStr)
	}

	// 工作目录
	cmd.Dir = cwd

	// 日志配置
	if err := SetLogConfig(cmd, t); err != nil {
		return err
	}

	// 环境变量配置
	envs := s.Envs
	if s.Type == constants.Configurable {
		// 数据库配置
		envs = append(envs, model.Env{Name: "CRAWLAB_MONGO_HOST", Value: viper.GetString("mongo.host")})
		envs = append(envs, model.Env{Name: "CRAWLAB_MONGO_PORT", Value: viper.GetString("mongo.port")})
		envs = append(envs, model.Env{Name: "CRAWLAB_MONGO_DB", Value: viper.GetString("mongo.db")})
		envs = append(envs, model.Env{Name: "CRAWLAB_MONGO_USERNAME", Value: viper.GetString("mongo.username")})
		envs = append(envs, model.Env{Name: "CRAWLAB_MONGO_PASSWORD", Value: viper.GetString("mongo.password")})
		envs = append(envs, model.Env{Name: "CRAWLAB_MONGO_AUTHSOURCE", Value: viper.GetString("mongo.authSource")})

		// 设置配置
		for envName, envValue := range s.Config.Settings {
			envs = append(envs, model.Env{Name: "CRAWLAB_SETTING_" + envName, Value: envValue})
		}
	}
	cmd = SetEnv(cmd, envs, t, s)

	// 起一个goroutine来监控进程
	ch := utils.TaskExecChanMap.ChanBlocked(t.Id)

	go FinishOrCancelTask(ch, cmd, t)

	// 启动进程
	if err := StartTaskProcess(cmd, t); err != nil {
		return err
	}

	// 同步等待进程完成
	if err := WaitTaskProcess(cmd, t); err != nil {
		return err
	}
	ch <- constants.TaskFinish
	return nil
}
