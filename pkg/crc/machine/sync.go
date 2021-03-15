package machine

import (
	"context"
	"errors"

	"github.com/code-ready/machine/libmachine/state"
	"golang.org/x/sync/semaphore"
)

type Synchronized struct {
	startLock  *semaphore.Weighted
	underlying Client
}

func NewSynchronizedMachine(machine Client) *Synchronized {
	return &Synchronized{
		startLock:  semaphore.NewWeighted(1),
		underlying: machine,
	}
}

func (s *Synchronized) IsProgressing() bool {
	if !s.startLock.TryAcquire(1) {
		return true
	}
	defer s.startLock.Release(1)
	return false
}

func (s *Synchronized) Delete() error {
	return s.underlying.Delete()
}

func (s *Synchronized) Start(context context.Context, startConfig StartConfig) (*StartResult, error) {
	if !s.startLock.TryAcquire(1) {
		return nil, errors.New("cluster is starting")
	}
	defer s.startLock.Release(1)
	return s.underlying.Start(context, startConfig)
}

func (s *Synchronized) Stop() (state.State, error) {
	return s.underlying.Stop()
}

func (s *Synchronized) GetName() string {
	return s.underlying.GetName()
}
func (s *Synchronized) Exists() (bool, error) {
	return s.underlying.Exists()
}

func (s *Synchronized) GetConsoleURL() (*ConsoleResult, error) {
	return s.underlying.GetConsoleURL()
}

func (s *Synchronized) IP() (string, error) {
	return s.underlying.IP()
}

func (s *Synchronized) PowerOff() error {
	return s.underlying.PowerOff()
}

func (s *Synchronized) Status() (*ClusterStatusResult, error) {
	return s.underlying.Status()
}

func (s *Synchronized) IsRunning() (bool, error) {
	return s.underlying.IsRunning()
}
