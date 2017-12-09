package add_docker_metadata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"

	"github.com/elastic/beats/libbeat/logp"
)

// Select Docker API version
const dockerAPIVersion = "1.22"

// Watcher reads docker events and keeps a list of known containers
type Watcher interface {
	// Start watching docker API for new containers
	Start() error

	// Container returns the running container with the given ID or nil if unknown
	Container(ID string) *Container

	// Containers returns the list of known containers
	Containers() map[string]*Container
}

type watcher struct {
	client             *client.Client
	ctx                context.Context
	stop               context.CancelFunc
	containers         map[string]*Container
	delcontainers      map[string] time.Time
	sync.RWMutex
	lastValidTimestamp int64
}

// Container info retrieved by the watcher
type Container struct {
	ID     string
	Name   string
	Image  string
	Labels map[string]string
	Env    map[string]string
}

type WatcherConstructor func(host string, tls *TLSConfig) (Watcher, error)

// NewWatcher returns a watcher running for the given settings
func NewWatcher(host string, tls *TLSConfig) (Watcher, error) {
	var httpClient *http.Client
	if tls != nil {
		options := tlsconfig.Options{
			CAFile:   tls.CA,
			CertFile: tls.Certificate,
			KeyFile:  tls.Key,
		}

		tlsc, err := tlsconfig.Client(options)
		if err != nil {
			return nil, err
		}

		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsc,
			},
		}
	}

	cli, err := client.NewClient(host, dockerAPIVersion, httpClient, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &watcher{
		client:     cli,
		ctx:        ctx,
		stop:       cancel,
		containers: make(map[string]*Container),
		delcontainers:make(map[string]time.Time),
	}, nil
}

// Container returns the running container with the given ID or nil if unknown
func (w *watcher) Container(ID string) *Container {
	return w.containers[ID]
}

// Containers returns the list of known containers
func (w *watcher) Containers() map[string]*Container {
	return w.containers
}

// Start watching docker API for new containers
func (w *watcher) Start() error {
	// Do initial scan of existing containers
	logp.Debug("docker", "Start docker containers scanner")
	w.lastValidTimestamp = time.Now().Unix()

	containers, err := w.client.ContainerList(w.ctx, types.ContainerListOptions{})
	if err != nil {
		return err
	}

	for _, c := range containers {
		container := &Container{
			ID:     c.ID,
			Name:   c.Names[0][1:], // Strip '/' from container names
			Image:  c.Image,
			Labels: c.Labels,
			Env:    make(map[string]string),
		}
		info, err := w.client.ContainerInspect(w.ctx, c.ID)
		if err == nil {
			for _, env := range info.Config.Env {
				kv := strings.SplitN(env, "=", 2)
				if len(kv) >= 2 {
					container.Env[kv[0]] = kv[1]
				}
			}
		}
		w.containers[c.ID] = container
		w.containers[container.Name] = container
	}

	go w.watch()
	go w.delay()

	return nil
}

func (w *watcher) delay() {
	for{
		time.Sleep(time.Second*3)
		now := time.Now()
		w.Lock()
		for k,v := range w.delcontainers{
			if now.Sub(v) > 20*time.Second{
				delete(w.containers, k)
				delete(w.delcontainers, k)
			}
		}
		w.Unlock()
	}

}

func (w *watcher) watch() {
	filters := filters.NewArgs()
	filters.Add("type", "container")

	options := types.EventsOptions{
		Since:   fmt.Sprintf("%d", w.lastValidTimestamp),
		Filters: filters,
	}

	for {
		events, errors := w.client.Events(w.ctx, options)

	WATCH:
		for {
			select {
			case event := <-events:
				logp.Debug("docker", "Got a new docker event: %v", event)
				w.lastValidTimestamp = event.Time

				// Add / update
				if event.Action == "create" || event.Action == "update" {
					name := event.Actor.Attributes["name"]
					image := event.Actor.Attributes["image"]
					delete(event.Actor.Attributes, "name")
					delete(event.Actor.Attributes, "image")
					container := &Container{
						ID:     event.Actor.ID,
						Name:   name,
						Image:  image,
						Labels: event.Actor.Attributes,
						Env:    make(map[string]string),
					}
					info, err := w.client.ContainerInspect(w.ctx, event.Actor.ID)
					if err == nil {
						for _, env := range info.Config.Env {
							kv := strings.SplitN(env, "=", 2)
							if len(kv) >= 2 {
								container.Env[kv[0]] = kv[1]
							}
						}
					}
					w.Lock()
					delete(w.delcontainers, container.ID)
					delete(w.delcontainers, container.Name)
					w.containers[container.ID] = container
					w.containers[container.Name] = container
					w.Unlock()
				}

				// Delete
				if event.Action == "die" || event.Action == "kill" {
					//delete(w.containers, event.Actor.ID)
					//delete(w.containers, event.Actor.Attributes["name"])
					now := time.Now()
					w.Lock()
					w.delcontainers[event.Actor.ID] = now
					w.delcontainers[event.Actor.Attributes["name"]] = now
					w.Unlock()
				}

			case err := <-errors:
				// Restart watch call
				logp.Err("Error watching for docker events: %v", err)
				time.Sleep(1 * time.Second)
				break WATCH

			case <-w.ctx.Done():
				logp.Debug("docker", "Watcher stopped")
				return
			}
		}
	}
}
