import os
import json
import time
import secrets
import threading
import urllib.request
from flask import request, g, jsonify

SERVICE_NAME = os.getenv("OTEL_SERVICE_NAME", "analytics-service")
CHAOS_STATE = {"enabled": False}


def _otlp_base_url():
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "").strip()
    if not endpoint:
        return ""
    if not endpoint.startswith("http://") and not endpoint.startswith("https://"):
        endpoint = f"http://{endpoint}"
    return endpoint.rstrip("/")


def parse_traceparent(header_val):
    if not header_val:
        return secrets.token_hex(16), ""
    parts = header_val.strip().split("-")
    if len(parts) == 4 and len(parts[1]) == 32 and len(parts[2]) == 16:
        return parts[1], parts[2]
    return secrets.token_hex(16), ""


def _post_otlp(path, payload):
    base = _otlp_base_url()
    if not base:
        return

    def _send():
        try:
            data = json.dumps(payload).encode("utf-8")
            req = urllib.request.Request(
                f"{base}{path}",
                data=data,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(req, timeout=1.5):
                pass
        except Exception:
            pass

    threading.Thread(target=_send, daemon=True).start()


def emit_otlp_span(trace_id, span_id, parent_span_id, name, kind, start_ns, end_ns, status_code=200, attrs=None):
    base = _otlp_base_url()
    if not base:
        return
    attributes = [
        {"key": "http.status_code", "value": {"intValue": str(status_code)}},
    ]
    for k, v in (attrs or {}).items():
        attributes.append({"key": k, "value": {"stringValue": str(v)}})

    otel_status = {"code": 2, "message": f"HTTP {status_code}"} if status_code >= 500 else {"code": 1}
    span_obj = {
        "traceId": trace_id,
        "spanId": span_id,
        "name": name,
        "kind": kind,
        "startTimeUnixNano": str(start_ns),
        "endTimeUnixNano": str(end_ns),
        "attributes": attributes,
        "status": otel_status,
    }
    if parent_span_id:
        span_obj["parentSpanId"] = parent_span_id

    payload = {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": [
                        {"key": "service.name", "value": {"stringValue": SERVICE_NAME}},
                        {"key": "deployment.environment", "value": {"stringValue": "production"}},
                    ]
                },
                "scopeSpans": [{"scope": {"name": SERVICE_NAME}, "spans": [span_obj]}],
            }
        ]
    }
    _post_otlp("/v1/traces", payload)


def emit_otlp_log(severity, message, status_code=200, route="", trace_id="", span_id=""):
    base = _otlp_base_url()
    if not base:
        return
    now_ns = str(time.time_ns())
    sev_num = 17 if severity == "ERROR" else (13 if severity == "WARN" else 9)
    payload = {
        "resourceLogs": [
            {
                "resource": {
                    "attributes": [
                        {"key": "service.name", "value": {"stringValue": SERVICE_NAME}},
                        {"key": "service", "value": {"stringValue": SERVICE_NAME}},
                    ]
                },
                "scopeLogs": [
                    {
                        "scope": {"name": SERVICE_NAME},
                        "logRecords": [
                            {
                                "timeUnixNano": now_ns,
                                "observedTimeUnixNano": now_ns,
                                "severityNumber": sev_num,
                                "severityText": severity,
                                "body": {"stringValue": message},
                                "attributes": [
                                    {"key": "service.name", "value": {"stringValue": SERVICE_NAME}},
                                    {"key": "http.route", "value": {"stringValue": route}},
                                    {"key": "http.status_code", "value": {"intValue": str(status_code)}},
                                    {"key": "trace_id", "value": {"stringValue": trace_id}},
                                    {"key": "span_id", "value": {"stringValue": span_id}},
                                ],
                            }
                        ],
                    }
                ],
            }
        ]
    }
    _post_otlp("/v1/logs", payload)


def instrument_flask_app(app):
    @app.route("/chaos", methods=["GET", "POST"])
    @app.route("/analytics-api/chaos", methods=["GET", "POST"])
    def chaos_handler():
        enable = request.args.get("enable", "").lower()
        if enable in ("true", "1"):
            CHAOS_STATE["enabled"] = True
            emit_otlp_log(
                "ERROR",
                f"[CHAOS_MODE_ENABLED] Injeção de falhas HTTP 500 ativada no {SERVICE_NAME}!",
                200,
                "/chaos",
            )
        elif enable in ("false", "0"):
            CHAOS_STATE["enabled"] = False
            emit_otlp_log(
                "INFO",
                f"[CHAOS_MODE_DISABLED] Injeção de falhas desativada no {SERVICE_NAME}.",
                200,
                "/chaos",
            )
        return jsonify({"service": SERVICE_NAME, "chaos_enabled": CHAOS_STATE["enabled"]}), 200

    @app.before_request
    def otel_before_request():
        g.start_ns = time.time_ns()
        trace_id, parent_span_id = parse_traceparent(request.headers.get("traceparent"))
        g.trace_id = trace_id
        g.parent_span_id = parent_span_id
        g.span_id = secrets.token_hex(8)

        if CHAOS_STATE["enabled"] and not request.path.endswith("/chaos"):
            return (
                jsonify({"error": f"Falha crítica simulada (Chaos Mode ativo no {SERVICE_NAME})"}),
                500,
            )
        return None

    @app.after_request
    def otel_after_request(response):
        start_ns = getattr(g, "start_ns", time.time_ns())
        end_ns = time.time_ns()
        trace_id = getattr(g, "trace_id", secrets.token_hex(16))
        span_id = getattr(g, "span_id", secrets.token_hex(8))
        parent_span_id = getattr(g, "parent_span_id", "")
        route = request.path
        status = response.status_code

        emit_otlp_span(
            trace_id=trace_id,
            span_id=span_id,
            parent_span_id=parent_span_id,
            name=f"{request.method} {route}",
            kind=2,
            start_ns=start_ns,
            end_ns=end_ns,
            status_code=status,
            attrs={"http.method": request.method, "http.route": route},
        )
        sev = "ERROR" if status >= 500 else "INFO"
        emit_otlp_log(
            sev,
            f"Requisição {request.method} {route} concluída ({status})",
            status,
            route,
            trace_id,
            span_id,
        )
        return response
