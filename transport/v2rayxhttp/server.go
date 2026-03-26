package v2rayxhttp

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/v2rayhttp"
	"github.com/sagernet/sing-box/transport/v2rayhttpupgrade"
	"github.com/sagernet/sing/common"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	aTLS "github.com/sagernet/sing/common/tls"
	sHTTP "github.com/sagernet/sing/protocol/http"
)

var _ adapter.V2RayServerTransport = (*Server)(nil)

type Server struct {
	ctx              context.Context
	logger           logger.ContextLogger
	tlsConfig        tls.ServerConfig
	handler          adapter.V2RayServerTransportHandler
	wrapper          adapter.V2RayServerTransport
	httpServer       *http.Server
	host             string
	path             string
	mode             string
	sessionPlacement string
	sessionKey       string
	seqPlacement     string
	seqKey           string
	uplinkPlacement  string
	headers          http.Header
	sessions         sync.Map
}

func NewServer(ctx context.Context, logger logger.ContextLogger, options option.V2RayXHTTPOptions, tlsConfig tls.ServerConfig, handler adapter.V2RayServerTransportHandler) (*Server, error) {
	mode, err := normalizeMode(options.Mode)
	if err != nil {
		return nil, err
	}
	sessionPlacement, err := normalizePlacement("session placement", options.SessionPlacement)
	if err != nil {
		return nil, err
	}
	seqPlacement, err := normalizePlacement("seq placement", options.SeqPlacement)
	if err != nil {
		return nil, err
	}
	uplinkPlacement, err := normalizeDataPlacement(options.UplinkDataPlacement)
	if err != nil {
		return nil, err
	}
	if mode == ModeStreamOne {
		transport, err := v2rayhttpupgrade.NewServer(ctx, logger, option.V2RayHTTPUpgradeOptions{
			Host:    options.Host,
			Path:    options.Path,
			Headers: options.Headers,
		}, tlsConfig, handler)
		if err != nil {
			return nil, err
		}
		return &Server{mode: mode, wrapper: transport}, nil
	}
	server := &Server{
		ctx:              ctx,
		logger:           logger,
		tlsConfig:        tlsConfig,
		handler:          handler,
		host:             options.Host,
		path:             normalizePath(options.Path),
		mode:             mode,
		sessionPlacement: sessionPlacement,
		sessionKey:       options.SessionKey,
		seqPlacement:     seqPlacement,
		seqKey:           options.SeqKey,
		uplinkPlacement:  uplinkPlacement,
		headers:          options.Headers.Build(),
	}
	server.httpServer = &http.Server{
		Handler:           server,
		ReadHeaderTimeout: C.TCPTimeout,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return log.ContextWithNewID(ctx)
		},
	}
	return server, nil
}

type wrapperTransport interface {
	adapter.V2RayServerTransport
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if s.httpServer == nil {
		writer.WriteHeader(http.StatusNotFound)
		return
	}
	if len(s.host) > 0 && request.Host != s.host {
		s.invalidRequest(writer, request, http.StatusBadRequest, E.New("bad host: ", request.Host))
		return
	}
	if !strings.HasPrefix(request.URL.Path, s.path) {
		s.invalidRequest(writer, request, http.StatusNotFound, E.New("bad path: ", request.URL.Path))
		return
	}
	for key, values := range s.headers {
		for _, value := range values {
			writer.Header().Set(key, value)
		}
	}
	writer.Header().Set("Cache-Control", "no-store")
	sessionID, seqText, ok := extractRequestMeta(request, s.path, s.sessionPlacement, s.seqPlacement, s.sessionKey, s.seqKey)
	if !ok {
		s.invalidRequest(writer, request, http.StatusBadRequest, E.New("invalid xhttp request metadata"))
		return
	}
	if sessionID == "" {
		s.invalidRequest(writer, request, http.StatusBadRequest, E.New("missing xhttp session"))
		return
	}
	session := s.session(sessionID)
	switch request.Method {
	case http.MethodGet:
		s.handleDownload(sessionID, session, writer, request)
	case http.MethodPost:
		s.handleUpload(session, writer, request, seqText)
	default:
		s.invalidRequest(writer, request, http.StatusMethodNotAllowed, E.New("unsupported method: ", request.Method))
	}
}

func (s *Server) handleDownload(sessionID string, session *serverSession, writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(http.StatusOK)
	writer.(http.Flusher).Flush()
	reader := session.reader()
	if request.Body != nil {
		defer request.Body.Close()
	}
	conn := v2rayhttp.NewHTTP2Wrapper(&v2rayhttp.ServerHTTPConn{
		HTTP2Conn: v2rayhttp.NewHTTPConn(reader, writer),
		Flusher:   writer.(http.Flusher),
	})
	source := sHTTP.SourceAddress(request)
	done := make(chan struct{})
	s.handler.NewConnectionEx(request.Context(), conn, source, M.Socksaddr{}, N.OnceClose(func(err error) {
		session.close()
		s.sessions.Delete(sessionID)
		close(done)
	}))
	select {
	case <-request.Context().Done():
	case <-done:
	}
	conn.CloseWrapper()
	_ = conn.Close()
}

func (s *Server) handleUpload(session *serverSession, writer http.ResponseWriter, request *http.Request, seqText string) {
	if s.mode == ModeStreamUp || (s.mode == ModeAuto && seqText == "") {
		if err := session.attachStream(request.Body); err != nil {
			_ = request.Body.Close()
			s.invalidRequest(writer, request, http.StatusConflict, err)
			return
		}
		writer.Header().Set("X-Accel-Buffering", "no")
		writer.WriteHeader(http.StatusOK)
		writer.(http.Flusher).Flush()
		select {
		case <-request.Context().Done():
		case <-session.done:
		}
		return
	}
	if seqText == "" {
		s.invalidRequest(writer, request, http.StatusBadRequest, E.New("missing xhttp seq"))
		return
	}
	seq, err := strconv.ParseInt(seqText, 10, 64)
	if err != nil {
		s.invalidRequest(writer, request, http.StatusBadRequest, E.New("invalid xhttp seq"))
		return
	}
	payload, err := extractPayloadFromRequest(request, s.uplinkPlacement, 1<<20)
	if err != nil {
		s.invalidRequest(writer, request, http.StatusBadRequest, E.Cause(err, "read request"))
		return
	}
	if err = session.push(seq, payload); err != nil {
		s.invalidRequest(writer, request, http.StatusConflict, err)
		return
	}
	writer.WriteHeader(http.StatusOK)
}

func (s *Server) invalidRequest(writer http.ResponseWriter, request *http.Request, statusCode int, err error) {
	if statusCode > 0 {
		writer.WriteHeader(statusCode)
	}
	s.logger.ErrorContext(request.Context(), E.Cause(err, "process connection from ", request.RemoteAddr))
}

func (s *Server) Network() []string {
	if s.wrapper != nil {
		return s.wrapper.Network()
	}
	return []string{N.NetworkTCP}
}

func (s *Server) Serve(listener net.Listener) error {
	if s.wrapper != nil {
		return s.wrapper.Serve(listener)
	}
	if s.tlsConfig != nil {
		if len(s.tlsConfig.NextProtos()) == 0 {
			s.tlsConfig.SetNextProtos([]string{"h2", "http/1.1"})
		}
		listener = aTLS.NewListener(listener, s.tlsConfig)
	}
	return s.httpServer.Serve(listener)
}

func (s *Server) ServePacket(listener net.PacketConn) error {
	if s.wrapper != nil {
		return s.wrapper.ServePacket(listener)
	}
	return os.ErrInvalid
}

func (s *Server) Close() error {
	if s.wrapper != nil {
		return s.wrapper.Close()
	}
	s.sessions.Range(func(key, value any) bool {
		value.(*serverSession).close()
		return true
	})
	return common.Close(common.PtrOrNil(s.httpServer))
}

func (s *Server) session(sessionID string) *serverSession {
	loaded, ok := s.sessions.Load(sessionID)
	if ok {
		return loaded.(*serverSession)
	}
	session := newServerSession(s.mode)
	actual, _ := s.sessions.LoadOrStore(sessionID, session)
	return actual.(*serverSession)
}

func (s *Server) wrapperOrNil() wrapperTransport {
	return s.wrapper
}
