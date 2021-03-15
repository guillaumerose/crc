package machine

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/code-ready/machine/libmachine/state"
	"github.com/stretchr/testify/assert"
)

func TestOneStartAtTheSameTime(t *testing.T) {
	isRunning := make(chan struct{}, 1)
	startCh := make(chan struct{}, 1)
	waitingMachine := &waitingMachine{
		isRunning:       isRunning,
		startCompleteCh: startCh,
	}
	syncMachine := NewSynchronizedMachine(waitingMachine)
	assert.False(t, syncMachine.IsProgressing())

	lock := &sync.WaitGroup{}
	lock.Add(1)
	go func() {
		defer lock.Done()
		_, err := syncMachine.Start(context.Background(), StartConfig{})
		assert.NoError(t, err)
	}()

	<-isRunning
	assert.True(t, syncMachine.IsProgressing())
	assert.Equal(t, waitingMachine.GetName(), syncMachine.GetName())
	_, err := syncMachine.Start(context.Background(), StartConfig{})
	assert.EqualError(t, err, "cluster is starting")

	startCh <- struct{}{}
	lock.Wait()

	assert.False(t, syncMachine.IsProgressing())
}

type waitingMachine struct {
	isRunning       chan struct{}
	startCompleteCh chan struct{}
}

func (m *waitingMachine) IsRunning() (bool, error) {
	return false, errors.New("not implemented")
}

func (m *waitingMachine) GetName() string {
	return "waiting machine"
}

func (m *waitingMachine) Delete() error {
	return errors.New("not implemented")
}

func (m *waitingMachine) Exists() (bool, error) {
	return false, errors.New("not implemented")
}

func (m *waitingMachine) GetConsoleURL() (*ConsoleResult, error) {
	return nil, errors.New("not implemented")
}

func (m *waitingMachine) IP() (string, error) {
	return "", errors.New("not implemented")
}

func (m *waitingMachine) PowerOff() error {
	return nil
}

func (m *waitingMachine) Start(context context.Context, startConfig StartConfig) (*StartResult, error) {
	m.isRunning <- struct{}{}
	<-m.startCompleteCh
	return &StartResult{
		Status:         state.Running,
		KubeletStarted: true,
	}, nil
}

func (m *waitingMachine) Status() (*ClusterStatusResult, error) {
	return nil, errors.New("not implemented")
}

func (m *waitingMachine) Stop() (state.State, error) {
	return state.Error, errors.New("not implemented")
}
