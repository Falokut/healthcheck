package healthcheck

import (
	"context"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

type HealthcheckResource interface {
	PingContext(ctx context.Context) error
}

type Manager struct {
	logger          *logrus.Logger
	toCheck         []HealthcheckResource
	listenPort      string
	additionalCheck func(context.Context) error
}

func NewHealthManager(logger *logrus.Logger,
	toCheck []HealthcheckResource, listenPort string, additionalCheck func(context.Context) error) Manager {
	return Manager{logger: logger,
		toCheck:         toCheck,
		listenPort:      listenPort,
		additionalCheck: additionalCheck}
}

func (m *Manager) RunHealthcheckEndpoint() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthcheck", m.healthcheckHandler)
	return http.ListenAndServe(":"+m.listenPort, mux)
}

func (m *Manager) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	m.logger.Print("Healthcheck started")
	defer m.logger.Print("Healthcheck done")

	wg := sync.WaitGroup{}
	wg.Add(len(m.toCheck))
	errorCh := make(chan error, 1)
	for _, resource := range m.toCheck {
		go func(resource HealthcheckResource) {
			defer wg.Done()
			err := resource.PingContext(r.Context())
			if err != nil {
				errorCh <- err
				return
			}
			select {
			case <-r.Context().Done():
				return
			default:
			}
		}(resource)
	}

	if m.additionalCheck != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.additionalCheck(r.Context()); err != nil {
				errorCh <- err
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-errorCh:
		m.logger.Errorf("healthcheck error: %s", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	case <-r.Context().Done():
		m.logger.Errorf("healthcheck error: timeout")
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	default:
		w.WriteHeader(http.StatusOK)
		return
	}
}
