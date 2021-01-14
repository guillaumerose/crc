package machine

import (
	"errors"

	"github.com/code-ready/machine/libmachine/state"
	"golang.org/x/sync/semaphore"
)

type Synchronized struct {
	error      error
	Lock       *semaphore.Weighted
	Underlying Client
}

func NewSynchronizedMachine(machine Client) *Synchronized {
	return &Synchronized{
		Lock:       semaphore.NewWeighted(1),
		Underlying: machine,
	}
}

func (s *Synchronized) IsProgressing() bool {
	if !s.Lock.TryAcquire(1) {
		return true
	}
	defer s.Lock.Release(1)
	return false
}

func (s *Synchronized) Delete() error {
	if !s.Lock.TryAcquire(1) {
		return s.error
	}
	defer s.Lock.Release(1)
	s.error = errors.New("cluster is being deleted")
	return s.Underlying.Delete()
}

func (s *Synchronized) Start(startConfig StartConfig) (*StartResult, error) {
	if !s.Lock.TryAcquire(1) {
		return nil, s.error
	}
	defer s.Lock.Release(1)
	s.error = errors.New("cluster is starting")
	return s.Underlying.Start(startConfig)
}

func (s *Synchronized) Stop() (state.State, error) {
	if !s.Lock.TryAcquire(1) {
		return state.Error, s.error
	}
	defer s.Lock.Release(1)
	s.error = errors.New("cluster is stopping")
	return s.Underlying.Stop()
}

func (s *Synchronized) GetName() string {
	return s.Underlying.GetName()
}
func (s *Synchronized) Exists() (bool, error) {
	return s.Underlying.Exists()
}

func (s *Synchronized) GetConsoleURL() (*ConsoleResult, error) {
	return s.Underlying.GetConsoleURL()
}

func (s *Synchronized) IP() (string, error) {
	return s.Underlying.IP()
}

func (s *Synchronized) PowerOff() error {
	return s.Underlying.PowerOff()
}

func (s *Synchronized) Status() (*ClusterStatusResult, error) {
	return s.Underlying.Status()
}
