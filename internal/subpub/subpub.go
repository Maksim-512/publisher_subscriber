package subpub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

const BufferSize = 100

var (
	ErrClosed        = errors.New("pubsub already closed")
	ErrNoSubscribers = errors.New("no such subscriber")
)

// MessageHandler - функция обратного вызова, которая обрабатывает сообщения, доставляемые подписчикам
type MessageHandler func(msg interface{})

type Subscription interface {
	// Unsubscribe приведет к удалению интерфейса
	Unsubscribe()
}

type SubPub interface {
	// Subscribe создает подписчика асинхронной очереди на заданную тему
	Subscribe(subject string, cb MessageHandler) (Subscription, error)

	// Publish публикует сообщение msg для заданной темы
	Publish(subject string, msg interface{}) error

	// Close завершает работу системы subpub
	// может быть заблокирована доставка данных до тех пор, пока контекст отменен
	Close(ctx context.Context) error
}

// Для одного подписчика
type subscriber struct {
	cb       MessageHandler
	msgCh    chan interface{}
	closedCh chan struct{}
}

// Для представления подписки
type subscription struct {
	implSubPub *pubSubImpl
	subject    string
	sub        *subscriber
}

type pubSubImpl struct {
	mu          sync.RWMutex
	subscribers map[string]map[*subscriber]struct{}
	closed      atomic.Bool
	closeOnce   sync.Once
	closedCh    chan struct{}
	wg          sync.WaitGroup
}

func NewSubPub() SubPub {
	return &pubSubImpl{
		subscribers: make(map[string]map[*subscriber]struct{}),
		closedCh:    make(chan struct{}),
	}
}

func (s *subscription) Unsubscribe() {
	s.implSubPub.mu.Lock()
	defer s.implSubPub.mu.Unlock()

	if s.implSubPub.closed.Load() {
		return
	}

	subs := s.implSubPub.subscribers[s.subject]

	_, exists := subs[s.sub]
	if exists {
		delete(subs, s.sub)
		close(s.sub.closedCh)
	}

}

func (i *pubSubImpl) Subscribe(subject string, cb MessageHandler) (Subscription, error) {
	if i.closed.Load() {
		return nil, ErrClosed
	}

	//select {
	//case <-i.closedCh:
	//	return nil, errors.New("pubsub closed")
	//default:
	//}

	sub := &subscriber{
		cb:       cb,
		msgCh:    make(chan interface{}, BufferSize),
		closedCh: make(chan struct{}),
	}

	i.mu.Lock()
	_, ok := i.subscribers[subject]
	if !ok {
		i.subscribers[subject] = make(map[*subscriber]struct{})
	}
	i.subscribers[subject][sub] = struct{}{}
	i.mu.Unlock()

	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		for {
			select {
			case msg := <-sub.msgCh:
				sub.cb(msg)
			case <-sub.closedCh:
				return
			case <-i.closedCh:
				return
			}
		}
	}()

	return &subscription{
		implSubPub: i,
		subject:    subject,
		sub:        sub,
	}, nil
}

func (i *pubSubImpl) Publish(subject string, msg interface{}) error {
	if i.closed.Load() {
		return ErrClosed
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	subs, ok := i.subscribers[subject]

	if !ok {
		return ErrNoSubscribers
	}

	for sub := range subs {
		select {
		case sub.msgCh <- msg: // Пытаемся отправить
		default:
		}
	}

	return nil
}

func (i *pubSubImpl) Close(ctx context.Context) error {
	if !i.closed.CompareAndSwap(false, true) {
		return nil
	}

	i.closeOnce.Do(func() {
		close(i.closedCh)
	})

	i.mu.Lock()
	for _, subject := range i.subscribers {
		for sub := range subject {
			close(sub.closedCh)
		}
	}
	i.subscribers = nil
	i.mu.Unlock()

	done := make(chan struct{})
	go func() {
		i.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
