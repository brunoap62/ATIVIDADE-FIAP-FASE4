package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	tracer       trace.Tracer
	otlpHTTPBase string
	chaosEnabled atomic.Bool
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

func initTelemetry(ctx context.Context, serviceName string) func(context.Context) error {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://otel-collector.observability.svc.cluster.local:4318"
	}
	otlpHTTPBase = strings.TrimRight(endpoint, "/")
	endpointHost := strings.TrimPrefix(strings.TrimPrefix(otlpHTTPBase, "http://"), "https://")

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpointHost),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Printf("Aviso: falha ao inicializar exporter OTLP (%v), seguindo com no-op", err)
		tracer = otel.Tracer(serviceName)
		return func(context.Context) error { return nil }
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("deployment.environment", "production"),
		),
	)
	if err != nil {
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer(serviceName)
	log.Printf("[OTel] OpenTelemetry inicializado para '%s' -> %s", serviceName, otlpHTTPBase)
	return tp.Shutdown
}

func emitOTLPLog(ctx context.Context, severity, message string, statusCode int, route string) {
	spanCtx := trace.SpanContextFromContext(ctx)
	traceID := ""
	spanID := ""
	if spanCtx.IsValid() {
		traceID = spanCtx.TraceID().String()
		spanID = spanCtx.SpanID().String()
	}

	logLine := fmt.Sprintf(`{"service":"evaluation-service","level":"%s","msg":%q,"route":%q,"status":%d,"trace_id":"%s","span_id":"%s"}`,
		severity, message, route, statusCode, traceID, spanID)
	log.Println(logLine)

	if otlpHTTPBase == "" {
		return
	}

	go func(line, sev, tID, sID string) {
		nowNano := fmt.Sprintf("%d", time.Now().UnixNano())
		payload := map[string]any{
			"resourceLogs": []map[string]any{
				{
					"resource": map[string]any{
						"attributes": []map[string]any{
							{"key": "service.name", "value": map[string]any{"stringValue": "evaluation-service"}},
							{"key": "deployment.environment", "value": map[string]any{"stringValue": "production"}},
						},
					},
					"scopeLogs": []map[string]any{
						{
							"logRecords": []map[string]any{
								{
									"timeUnixNano": nowNano,
									"severityText": sev,
									"body":         map[string]any{"stringValue": line},
									"traceId":      tID,
									"spanId":       sID,
								},
							},
						},
					},
				},
			},
		}
		b, err := json.Marshal(payload)
		if err != nil {
			return
		}
		req, err := http.NewRequest(http.MethodPost, otlpHTTPBase+"/v1/logs", bytes.NewReader(b))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}(logLine, severity, traceID, spanID)
}

func (a *App) otelMiddleware(route string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqCtx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		if tracer == nil {
			tracer = otel.Tracer("evaluation-service")
		}

		reqCtx, span := tracer.Start(reqCtx, fmt.Sprintf("%s %s", r.Method, route),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", route),
				attribute.String("http.target", r.URL.Path),
			),
		)
		defer span.End()

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		if chaosEnabled.Load() && route != "/chaos" {
			rec.WriteHeader(http.StatusInternalServerError)
			_, _ = rec.Write([]byte(`{"error":"[CHAOS_FAULT_INJECTION] Falha crítica simulada no evaluation-service"}`))
			span.SetAttributes(attribute.Int("http.status_code", http.StatusInternalServerError))
			span.SetStatus(codes.Error, "Simulated 500 Internal Server Error for Self-Healing test")
			emitOTLPLog(reqCtx, "ERROR", "Falha crítica HTTP 500 no evaluation-service (Chaos Mode ativo)", http.StatusInternalServerError, route)
			return
		}

		next(rec, r.WithContext(reqCtx))

		span.SetAttributes(attribute.Int("http.status_code", rec.statusCode))
		if rec.statusCode >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.statusCode))
			emitOTLPLog(reqCtx, "ERROR", fmt.Sprintf("Erro HTTP %d em %s", rec.statusCode, route), rec.statusCode, route)
		} else {
			emitOTLPLog(reqCtx, "INFO", fmt.Sprintf("Requisição %s %s concluída (%d)", r.Method, route, rec.statusCode), rec.statusCode, route)
		}
	}
}

func (a *App) chaosHandler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("enable") {
	case "true", "1":
		chaosEnabled.Store(true)
		emitOTLPLog(r.Context(), "ERROR", "[CHAOS_MODE_ENABLED] Injeção de falhas HTTP 500 ativada no evaluation-service!", http.StatusOK, "/chaos")
	case "false", "0":
		chaosEnabled.Store(false)
		emitOTLPLog(r.Context(), "INFO", "[CHAOS_MODE_DISABLED] Injeção de falhas desativada no evaluation-service.", http.StatusOK, "/chaos")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"service":       "evaluation-service",
		"chaos_enabled": chaosEnabled.Load(),
	})
}
