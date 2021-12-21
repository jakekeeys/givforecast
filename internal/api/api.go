package api

import (
	"ge-charge-optimiser/internal/projector"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

const dateFormat = "2006-01-02"

type Server struct {
	p *projector.Projector
}

func NewServer(p *projector.Projector) *Server {
	return &Server{p: p}
}

func (s *Server) Project(c *gin.Context) {
	ds := c.Query("date")
	if ds == "" {
		ds = time.Now().Format(dateFormat)
	}

	d, err := time.Parse(dateFormat, ds)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	today := time.Now().Truncate(time.Hour * 24)
	if d.Before(today) {
		c.String(http.StatusBadRequest, "date must be today or < 7 days in the future")
		return
	}

	if d.After(today.AddDate(0, 0, 6)) {
		c.String(http.StatusBadRequest, "date must be today or < 7 days in the future")
		return
	}

	projection, err := s.p.Project(d)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, projection)
	return
}

func (s *Server) Config(c *gin.Context) {
	config := s.p.GetConfig()

	c.JSON(http.StatusOK, config)
	return
}
