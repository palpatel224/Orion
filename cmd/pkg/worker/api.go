package worker

import(
	"github.com/go-chi/chi"
	"net/http"
	"fmt"
)

type Api struct {
	Address string
	Port int
	Worker *Worker
	Router *chi.Mux
}

type ErrResponse struct {
	HTTPStatusCode int
	Message string
}

func (a *Api) Start(){
	a.initRouter()
	http.ListenAndServe(fmt.Sprintf("%s:%d",a.Address,a.Port),a.Router)
}

func (a *Api) initRouter(){
	a.Router = chi.NewRouter()
	a.Router.Get("/health", a.HealthHandler)
	a.Router.Post("/bandwidth-receive",a.BandwidthRecieverHandler)
	a.Router.Post("/measure-bandwidth",a.MeasureBandwidthHandler)
	a.Router.Route("/tasks",func(r chi.Router){
		r.Post("/",a.StartTaskHandler)
		r.Get("/",a.GetTasksHandler)
		r.Route("/{taskID}",func(r chi.Router){//subroute
			r.Delete("/",a.StopTaskHandler)
		})
	})
	a.Router.Route("/stats",func(r chi.Router){
		r.Get("/",a.GetStatsHandler)
	})
}

