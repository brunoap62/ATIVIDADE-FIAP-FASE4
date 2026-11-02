import os
import sys
import psycopg2
import requests
import json
from psycopg2.extras import RealDictCursor, Json
from psycopg2.pool import SimpleConnectionPool
from flask import Flask, request, jsonify
from dotenv import load_dotenv
from functools import wraps
import logging
import time
from telemetry import instrument_flask_app, get_outbound_trace_headers, emit_otlp_span

# Configura o logging
logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

# Carrega .env para desenvolvimento local
load_dotenv()

app = Flask(__name__)
instrument_flask_app(app)

# --- Configuração ---
DATABASE_URL = os.getenv("DATABASE_URL")
AUTH_SERVICE_URL = os.getenv("AUTH_SERVICE_URL")

if not DATABASE_URL or not AUTH_SERVICE_URL:
    log.critical("Erro: DATABASE_URL e AUTH_SERVICE_URL devem ser definidos.")
    sys.exit(1)

# --- Pool de Conexão com o Banco & Auto-Migração ---
try:
    pool = SimpleConnectionPool(1, 5, dsn=DATABASE_URL)
    log.info("Pool de conexões com o PostgreSQL (targeting) inicializado.")
except psycopg2.OperationalError as e:
    log.critical(f"Erro fatal ao conectar ao PostgreSQL: {e}")
    sys.exit(1)

def init_db(connection_pool):
    """Executa a auto-migração criando a tabela targeting_rules e triggers caso não existam"""
    ddl_statements = """
    CREATE TABLE IF NOT EXISTS targeting_rules (
        id SERIAL PRIMARY KEY,
        flag_name VARCHAR(100) UNIQUE NOT NULL,
        is_enabled BOOLEAN NOT NULL DEFAULT true,
        rules JSONB NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
    );

    CREATE OR REPLACE FUNCTION trigger_set_timestamp()
    RETURNS TRIGGER AS $$
    BEGIN
      NEW.updated_at = NOW();
      RETURN NEW;
    END;
    $$ LANGUAGE plpgsql;

    DROP TRIGGER IF EXISTS set_timestamp ON targeting_rules;

    CREATE TRIGGER set_timestamp
    BEFORE UPDATE ON targeting_rules
    FOR EACH ROW
    EXECUTE PROCEDURE trigger_set_timestamp();
    """
    conn = None
    try:
        conn = connection_pool.getconn()
        with conn.cursor() as cur:
            cur.execute(ddl_statements)
        conn.commit()
        log.info("Auto-Migration concluída com sucesso: tabela 'targeting_rules' e triggers verificados/criados.")
    except Exception as e:
        if conn: conn.rollback()
        log.critical(f"Erro fatal na auto-migração do banco de dados (targeting): {e}")
        sys.exit(1)
    finally:
        if conn: connection_pool.putconn(conn)

# Executa auto-migração na inicialização
init_db(pool)

# --- Middleware de Autenticação (Idêntico ao flag-service) ---
def require_auth(f):
    """ Middleware para validar a chave de API contra o auth-service """
    @wraps(f)
    def decorated(*args, **kwargs):
        auth_header = request.headers.get("Authorization")
        if not auth_header:
            return jsonify({"error": "Authorization header obrigatório"}), 401
        
        try:
            validate_url = f"{AUTH_SERVICE_URL}/validate"
            trace_headers, trace_id, c_span_id, p_span_id = get_outbound_trace_headers()
            headers = {"Authorization": auth_header, **trace_headers}
            start_ns = time.time_ns()
            response = requests.get(validate_url, headers=headers, timeout=3)
            emit_otlp_span(
                trace_id=trace_id,
                span_id=c_span_id,
                parent_span_id=p_span_id,
                name="GET auth-service/validate",
                kind=3,
                start_ns=start_ns,
                end_ns=time.time_ns(),
                status_code=response.status_code,
                attrs={"peer.service": "auth-service", "http.url": validate_url},
            )

            if response.status_code != 200:
                log.warning(f"Falha na validação da chave (status: {response.status_code})")
                return jsonify({"error": "Chave de API inválida"}), 401
        
        except requests.exceptions.Timeout:
            log.error("Timeout ao conectar com o auth-service")
            return jsonify({"error": "Serviço de autenticação indisponível (timeout)"}), 504 # Gateway Timeout
        except requests.exceptions.RequestException as e:
            log.error(f"Erro ao conectar com o auth-service: {e}")
            return jsonify({"error": "Serviço de autenticação indisponível"}), 503 # Service Unavailable

        return f(*args, **kwargs)
    return decorated

# --- Endpoints da API ---

@app.route('/health')
def health():
    return jsonify({"status": "ok targeting-service"})

@app.route('/rules', methods=['POST'])
@app.route('/', methods=['POST'])
@require_auth
def create_rule():
    """ Cria uma nova regra de segmentação para uma flag """
    data = request.get_json()
    if not data or 'flag_name' not in data or 'rules' not in data:
        return jsonify({"error": "'flag_name' e 'rules' (JSON) são obrigatórios"}), 400
    
    flag_name = data['flag_name']
    rules_obj = data['rules'] # O objeto JSON
    is_enabled = data.get('is_enabled', True)
    
    conn = None
    cur = None
    try:
        conn = pool.getconn()
        cur = conn.cursor(cursor_factory=RealDictCursor)
        cur.execute(
            "INSERT INTO targeting_rules (flag_name, is_enabled, rules, created_at, updated_at) "
            "VALUES (%s, %s, %s, NOW(), NOW()) RETURNING *",
            (flag_name, is_enabled, Json(rules_obj)) # Usa Json() para serializar
        )
        new_rule = cur.fetchone()
        conn.commit()
        log.info(f"Regra para '{flag_name}' criada com sucesso.")
        return jsonify(new_rule), 201
    except psycopg2.IntegrityError:
        if conn: conn.rollback()
        log.warning(f"Tentativa de criar regra duplicada: '{flag_name}'")
        return jsonify({"error": f"Regra para a flag '{flag_name}' já existe"}), 409
    except Exception as e:
        if conn: conn.rollback()
        log.error(f"Erro ao criar regra: {e}")
        return jsonify({"error": "Erro interno do servidor", "details": str(e)}), 500
    finally:
        if cur: cur.close()
        if conn: pool.putconn(conn)

@app.route('/rules/<string:flag_name>', methods=['GET'])
@app.route('/<string:flag_name>', methods=['GET'])
@require_auth
def get_rule(flag_name):
    """ Busca uma regra de segmentação pelo nome da flag """
    conn = None
    cur = None
    try:
        conn = pool.getconn()
        cur = conn.cursor(cursor_factory=RealDictCursor)
        cur.execute("SELECT * FROM targeting_rules WHERE flag_name = %s", (flag_name,))
        rule = cur.fetchone()
        if not rule:
            return jsonify({"error": "Regra não encontrada"}), 404
        return jsonify(rule)
    except Exception as e:
        log.error(f"Erro ao buscar regra '{flag_name}': {e}")
        return jsonify({"error": "Erro interno do servidor", "details": str(e)}), 500
    finally:
        if cur: cur.close()
        if conn: pool.putconn(conn)

@app.route('/rules/<string:flag_name>', methods=['PUT'])
@app.route('/<string:flag_name>', methods=['PUT'])
@require_auth
def update_rule(flag_name):
    """ Atualiza a regra de segmentação de uma flag """
    data = request.get_json()
    if not data:
        return jsonify({"error": "Corpo da requisição obrigatório"}), 400

    fields = []
    values = []
    
    if 'rules' in data:
        fields.append("rules = %s")
        values.append(Json(data['rules'])) # Serializa o JSON
    if 'is_enabled' in data:
        fields.append("is_enabled = %s")
        values.append(data['is_enabled'])
    
    if not fields:
        return jsonify({"error": "Pelo menos um campo ('rules', 'is_enabled') é obrigatório"}), 400
    
    values.append(flag_name) # Adiciona o 'flag_name' para a cláusula WHERE
    
    query = f"UPDATE targeting_rules SET {', '.join(fields)} WHERE flag_name = %s RETURNING *"
    
    conn = None
    cur = None
    try:
        conn = pool.getconn()
        cur = conn.cursor(cursor_factory=RealDictCursor)
        cur.execute(query, tuple(values))
        
        if cur.rowcount == 0:
            return jsonify({"error": "Regra não encontrada"}), 404
            
        updated_rule = cur.fetchone()
        conn.commit()
        log.info(f"Regra para '{flag_name}' atualizada com sucesso.")
        return jsonify(updated_rule), 200
    except Exception as e:
        if conn: conn.rollback()
        log.error(f"Erro ao atualizar regra '{flag_name}': {e}")
        return jsonify({"error": "Erro interno do servidor", "details": str(e)}), 500
    finally:
        if cur: cur.close()
        if conn: pool.putconn(conn)

@app.route('/rules/<string:flag_name>', methods=['DELETE'])
@app.route('/<string:flag_name>', methods=['DELETE'])
@require_auth
def delete_rule(flag_name):
    """ Deleta a regra de segmentação de uma flag """
    conn = None
    cur = None
    try:
        conn = pool.getconn()
        cur = conn.cursor()
        cur.execute("DELETE FROM targeting_rules WHERE flag_name = %s", (flag_name,))
        
        if cur.rowcount == 0:
            return jsonify({"error": "Regra não encontrada"}), 404
            
        conn.commit()
        log.info(f"Regra para '{flag_name}' deletada com sucesso.")
        return "", 204 # 204 No Content
    except Exception as e:
        if conn: conn.rollback()
        log.error(f"Erro ao deletar regra '{flag_name}': {e}")
        return jsonify({"error": "Erro interno do servidor", "details": str(e)}), 500
    finally:
        if cur: cur.close()
        if conn: pool.putconn(conn)

if __name__ == '__main__':
    port = int(os.getenv("PORT", 8003))
    app.run(host='0.0.0.0', port=port, debug=False)