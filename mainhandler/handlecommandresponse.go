package mainhandler

import (
	"time"

	"github.com/golang/glog"
)

type HandleCommandResponseCallBack func(payload interface{}) (bool, *time.Duration)

const (
	MaxLimitationInsertToCommandResponseChannelGoRoutine = 10
)

const (
	KubascapeResponse string = "KubascapeResponse"
)

type CommandResponseData struct {
	commandName                        string
	isCommandResponseNeedToBeRehandled bool
	nextHandledTime                    *time.Duration
	handleCallBack                     HandleCommandResponseCallBack
	payload                            interface{}
}

type timerData struct {
	timer   *time.Timer
	payload interface{}
}

type commandResponseChannelData struct {
	commandResponseChannel                  *chan *CommandResponseData
	limitedGoRoutinesCommandResponseChannel *chan *timerData
}

func createNewCommandResponseData(commandName string, cb HandleCommandResponseCallBack, payload interface{}, nextHandledTime *time.Duration) *CommandResponseData {
	return &CommandResponseData{
		commandName:     commandName,
		handleCallBack:  cb,
		payload:         payload,
		nextHandledTime: nextHandledTime,
	}
}

func insertNewCommandResponseData(commandResponseChannel *commandResponseChannelData, data *CommandResponseData) {
	glog.Infof("insert new data of %s to command response channel", data.commandName)
	timer := time.NewTimer(*data.nextHandledTime)
	*commandResponseChannel.limitedGoRoutinesCommandResponseChannel <- &timerData{
		timer:   timer,
		payload: data,
	}
}

func (mainHandler *MainHandler) waitTimerTofinishAndInsert(data *timerData) {
	<-data.timer.C
	*mainHandler.commandResponseChannel.commandResponseChannel <- data.payload.(*CommandResponseData)
}

func (mainHandler *MainHandler) handleLimitedGoroutineOfCommandsResponse() {
	for {
		tData := <-*mainHandler.commandResponseChannel.limitedGoRoutinesCommandResponseChannel
		mainHandler.waitTimerTofinishAndInsert(tData)
	}
}

func (mainHandler *MainHandler) createInsertCommandsResponseThreadPool() {
	for i := 0; i < MaxLimitationInsertToCommandResponseChannelGoRoutine; i++ {
		go mainHandler.handleLimitedGoroutineOfCommandsResponse()
	}
}

func (mainHandler *MainHandler) handleCommandResponse() {
	mainHandler.createInsertCommandsResponseThreadPool()
	for {
		data := <-*mainHandler.commandResponseChannel.commandResponseChannel
		glog.Infof("handle command response %s", data.commandName)
		data.isCommandResponseNeedToBeRehandled, data.nextHandledTime = data.handleCallBack(data.payload)
		glog.Infof("%s is need to be rehandled: %v", data.commandName, data.isCommandResponseNeedToBeRehandled)
		if data.isCommandResponseNeedToBeRehandled {
			insertNewCommandResponseData(mainHandler.commandResponseChannel, data)
		}
	}
}
