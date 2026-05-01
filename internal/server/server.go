package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/avlek/ds123/internal/chat"
	"github.com/avlek/ds123/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

type Server struct {
	e       *echo.Echo
	Chat    *chat.Chat
	Storage *storage.Storage
}

func New(pool *pgxpool.Pool, apiKey, weatherAPIKey string) *Server {
	e := echo.New()
	s := storage.New(pool)

	return &Server{
		e:    e,
		Chat: chat.New(apiKey, weatherAPIKey, s),
	}
}

func (s *Server) SetRoutes() {
	s.e.POST("/chat", func(c echo.Context) error {
		var params chat.Request
		err2 := c.Bind(&params)
		if err2 != nil {
			return c.NoContent(http.StatusBadRequest)
		}

		reply, err2 := s.Chat.SendMessage(c.Request().Context(), params.SessionID, params.Message)
		if err2 != nil {
			return c.NoContent(http.StatusInternalServerError)
		}

		return c.JSON(http.StatusOK, chat.Response{Reply: reply})
	})
}

func (s *Server) Run() error {
	s.SetRoutes()

	ctx2, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	serverErr := make(chan error, 1)
	go func() {
		if err2 := s.e.Start(":8080"); err2 != nil && !errors.Is(err2, http.ErrServerClosed) {
			serverErr <- err2
		}
		close(serverErr)
	}()

	select {
	case <-ctx2.Done():
		log.Println("signal received, shutting down")
	case err2 := <-serverErr:
		log.Println("server crashed:", err2)
	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.e.Shutdown(shutdownCtx); err != nil {
		log.Println("shutdown error:", err)
	}

	return nil
}
