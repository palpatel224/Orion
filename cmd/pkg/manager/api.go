package manager

import (
	"fmt"
	"net/http"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Api struct {
	Address string
	Port    int
	Manager *Manager
	Router  *chi.Mux
	HTTPCli *http.Client
}

type ErrResponse struct {
	HTTPStatusCode int `json:"status"`
	Message string `json:"message"`
}

func (a *Api) HTTPClient() *http.Client{
	if a.HTTPCli!=nil{
		return a.HTTPCli
	}
	return http.DefaultClient
}

func (a *Api) initRouter() {
	a.Router = chi.NewRouter()
	a.Router.Get("/status",a.StatusHandler)
	a.Router.Post("/metrics", func(w http.ResponseWriter, r *http.Request) {
		promhttp.Handler().ServeHTTP(w, r)})
	a.Router.Route("/tasks", func(r chi.Router) {
		r.Post("/", a.StartTaskHandler)
		r.Get("/", a.GetTasksHandler)
		r.Route("/{taskID}", func(r chi.Router) {
			r.Delete("/", a.StopTaskHandler)
		})
	})
	a.Router.Route("/app",func(r chi.Router){
		r.Post("/",a.CreateAppHandler)
	})
	a.Router.Route("/workers",func(r chi.Router){
		r.Post("/",a.RegisterWorkerHandler)
		r.Route("/{workerID}",func(r chi.Router){
			r.Put("/heartbeat",a.HeartbeatHandler)
		})
	})
	a.Router.Get("/nodes",a.GetNodesHandler)
}

func (a *Api) Start() {
	a.initRouter()
	http.ListenAndServe(fmt.Sprintf("%s:%d", a.Address, a.Port), a.Router)
}