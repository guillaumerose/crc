package machine

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/code-ready/machine/libmachine/state"
)

func TestSyncOperations(t *testing.T) {
	isRunning := make(chan struct{}, 1)
	stopCh := make(chan struct{}, 1)
	waitingMachine := &waitingMachine{
		isRunning: isRunning,
		stopCh:    stopCh,
	}
	syncMachine := NewSynchronizedMachine(waitingMachine)
	assert.False(t, syncMachine.IsProgressing())

	lock := &sync.WaitGroup{}
	lock.Add(1)
	go func() {
		defer lock.Done()
		assert.NoError(t, syncMachine.Delete())
	}()

	<-isRunning
	assert.True(t, syncMachine.IsProgressing())
	assert.Equal(t, waitingMachine.GetName(), syncMachine.GetName())
	assert.EqualError(t, syncMachine.Delete(), "cluster is being deleted")
	_, err := syncMachine.Stop()
	assert.EqualError(t, err, "cluster is being deleted")

	stopCh <- struct{}{}
	lock.Wait()

	assert.False(t, syncMachine.IsProgressing())
}

type waitingMachine struct {
	isRunning chan struct{}
	stopCh    chan struct{}
}

func (m *waitingMachine) GetName() string {
	return "waiting machine"
}

func (m *waitingMachine) Delete() error {
	m.isRunning <- struct{}{}
	<-m.stopCh
	return nil
}

func (m *waitingMachine) Exists() (bool, error) {
	return false, nil
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

func (m *waitingMachine) Start(startConfig StartConfig) (*StartResult, error) {
	return nil, errors.New("not implemented")
}

func (m *waitingMachine) Status() (*ClusterStatusResult, error) {
	return nil, errors.New("not implemented")
}

func (m *waitingMachine) Stop() (state.State, error) {
	return state.Error, errors.New("not implemented")
}
