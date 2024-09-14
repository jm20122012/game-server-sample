package server

import (
	"bufio"
	"context"
	"errors"
	"gameserver/internal/client"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	GAME_SERVER_PORT string = "7000"
)

type clientMsg struct {
	client net.Conn
	msg    []byte
}

type Server struct {
	ctx             context.Context
	cancelFunc      context.CancelFunc
	logger          *slog.Logger
	clients         map[string]*client.Client
	clientMsgBuffer chan clientMsg
	serverPort      string
	tcpListener     net.Listener
	mutex           sync.RWMutex
}

func NewServer(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *slog.Logger,
	port string,
	listener net.Listener,
) *Server {
	return &Server{
		ctx:             ctx,
		cancelFunc:      cancel,
		logger:          logger,
		clients:         make(map[string]*client.Client),
		clientMsgBuffer: make(chan clientMsg, 64000),
		serverPort:      port,
		tcpListener:     listener,
		mutex:           sync.RWMutex{},
	}
}

func (s *Server) StartServer() {
	defer close(s.clientMsgBuffer)
	s.logger.Info("Starting server tick")

	t := time.NewTicker(time.Second * 1)
	defer t.Stop()

	go s.serverTick(t)

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("Exiting main loop")
			return
		default:
			s.logger.Info("Listening for connections...")

			conn, err := s.tcpListener.Accept()
			if err != nil {
				if s.ctx.Err() != nil {
					// Context has been cancelled; exit the loop
					s.logger.Info("Listener closed, exiting main loop")
					return
				}
				s.logger.Error("error accepting TCP connection", "error", err)
				continue
			}

			s.logger.Info("Connection received", "conn", conn)
			go s.handleConnection(conn)
		}
	}
}

func (s *Server) registerClient(id string, conn net.Conn) error {
	s.logger.Info("Registering client", "clientID", id, "conn", conn)
	s.mutex.RLock()
	_, ok := s.clients[id]
	s.mutex.RUnlock()

	if ok {
		e := errors.New("error: client already exists in client map")
		return e
	}

	c := client.NewClient(conn)
	s.mutex.Lock()
	s.clients[id] = c
	s.mutex.Unlock()

	return nil
}

func (s *Server) unregisterClient(id string, conn net.Conn) error {
	s.logger.Info("Unregistering client", "clientID", id, "conn", conn)
	s.mutex.RLock()
	_, ok := s.clients[id]
	s.mutex.RUnlock()

	if !ok {
		e := errors.New("error: client does not exist in client map")
		return e
	}

	s.mutex.Lock()
	delete(s.clients, id)
	s.mutex.Unlock()

	return nil
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	newId := uuid.NewString()
	err := s.registerClient(newId, conn)
	if err != nil {
		s.logger.Error("error registering client", "error", err)
		return
	}
	defer s.unregisterClient(newId, conn)

	msgChannel := make(chan []byte, 1000)
	defer close(msgChannel)
	reader := bufio.NewReader(conn)

	connCtx, connCancel := context.WithCancel(s.ctx)
	defer connCancel()

	// Goroutine to read messages from the client
	go func() {
		for {
			msg, err := reader.ReadBytes(0x04) // 0x04 is the ASCII "End of Transmission" (EOT) character
			if err != nil {
				// Handle the error
				if errors.Is(err, net.ErrClosed) {
					s.logger.Info("Connection closed, exiting reader goroutine", "connection", conn)
				} else if err == io.EOF {
					s.logger.Info("EOF received, exiting reader goroutine", "connection", conn)
				} else {
					s.logger.Error("Error reading message from client", "error", err)
				}
				connCancel()
				return
			}

			// **Check for empty messages**
			if len(msg) == 0 {
				// Empty message received, skip processing
				continue
			}

			// 0x17 is the ASCII "End of Transmission Block" character
			if msg[0] == 0x17 {
				s.logger.Info("Client sent end of transmission block signal - exiting goroutine")
				connCancel()
				return
			}

			select {
			case msgChannel <- msg:
			case <-connCtx.Done():
				return
			}
		}
	}()

	// Goroutine to close the connection on context cancellation
	// go func() {
	// 	<-s.ctx.Done()
	// 	s.logger.Info("Context canceled, closing connection", "clientID", newId, "conn", conn)
	// 	conn.Close() // This will unblock the reader
	// }()

	// Main loop to process messages
	for {
		select {
		case msg, ok := <-msgChannel:
			if !ok {
				return
			}
			// s.logger.Info("Message received from client - adding to queue", "conn", conn, "msg", msg, "msgLength", len(msg))
			s.clientMsgBuffer <- clientMsg{
				client: conn,
				msg:    msg,
			}
		// case <-s.ctx.Done():
		// 	s.logger.Info("Context canceled in client handler main loop, exiting", "connection", conn)
		// 	return
		case <-connCtx.Done():
			s.logger.Info("Context canceled in client handler main loop, exiting", "connection", conn)
			return
		}
	}
}

func (s *Server) serverTick(t *time.Ticker) {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Warn("Server context cancel detected in server tick - exiting")
			return
		case <-t.C: // On every tick
			// s.logger.Info("Executing server tick")
			s.mutex.RLock()
			s.logger.Debug("Client count", "count", len(s.clients))
			s.mutex.RUnlock()
			// Non-blocking loop to process all messages currently in the buffer
			for {
				select {
				case msg := <-s.clientMsgBuffer:
					s.logger.Debug("processing msg", "msg", msg.msg)
					data := strings.Split(strings.Trim(string(msg.msg), "\x04"), ",")
					s.logger.Debug("Client msg data", "data", data)
				default:
					// Exit the loop when there are no more messages
					goto EndMessageProcessing
				}
			}
		}
	EndMessageProcessing:
	}
}
