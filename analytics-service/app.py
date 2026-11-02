import os
import sys
import threading
import json
import uuid
import time
import secrets
import logging
import boto3
from botocore.exceptions import NoCredentialsError, ClientError
from flask import Flask, jsonify
from dotenv import load_dotenv
from telemetry import (
    instrument_flask_app,
    parse_traceparent,
    emit_otlp_span,
    emit_otlp_log,
)

# Configura o logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
log = logging.getLogger(__name__)

# Carrega .env para desenvolvimento local
load_dotenv()

# --- Configuração ---
AWS_REGION = os.getenv("AWS_REGION")
SQS_QUEUE_URL = os.getenv("AWS_SQS_URL")
DYNAMODB_TABLE_NAME = os.getenv("AWS_DYNAMODB_TABLE")

if not all([AWS_REGION, SQS_QUEUE_URL, DYNAMODB_TABLE_NAME]):
    log.critical("Erro: AWS_REGION, AWS_SQS_URL, e AWS_DYNAMODB_TABLE devem ser definidos.")
    sys.exit(1)

# --- Clientes Boto3 ---
# Criamos a sessão uma vez
try:
    session = boto3.Session(region_name=AWS_REGION)
    sqs_client = session.client("sqs")
    dynamodb_client = session.client("dynamodb")
    log.info(f"Clientes Boto3 inicializados na região {AWS_REGION}")
except NoCredentialsError:
    log.critical("Credenciais da AWS não encontradas. Verifique seu ambiente.")
    sys.exit(1)
except Exception as e:
    log.critical(f"Erro ao inicializar o Boto3: {e}")
    sys.exit(1)


# --- SQS Worker ---

def process_message(message):
    """ Processa uma única mensagem SQS e a insere no DynamoDB propagando W3C TraceContext """
    start_ns = time.time_ns()
    msg_attrs = message.get("MessageAttributes", {})
    tp_val = msg_attrs.get("traceparent", {}).get("StringValue", "")
    trace_id, parent_span_id = parse_traceparent(tp_val)
    consumer_span_id = secrets.token_hex(8)

    try:
        log.info(f"Processando mensagem ID: {message['MessageId']}")
        body = json.loads(message['Body'])

        # Gera um ID único para o item no DynamoDB
        event_id = str(uuid.uuid4())

        # Constrói o item no formato do DynamoDB
        item = {
            'event_id': {'S': event_id},
            'user_id': {'S': body['user_id']},
            'flag_name': {'S': body['flag_name']},
            'result': {'BOOL': body['result']},
            'timestamp': {'S': body['timestamp']}
        }

        # Insere no DynamoDB registrando Span de Cliente
        ddb_start = time.time_ns()
        ddb_span_id = secrets.token_hex(8)
        dynamodb_client.put_item(
            TableName=DYNAMODB_TABLE_NAME,
            Item=item
        )
        emit_otlp_span(
            trace_id=trace_id,
            span_id=ddb_span_id,
            parent_span_id=consumer_span_id,
            name="DynamoDB PutItem",
            kind=3,
            start_ns=ddb_start,
            end_ns=time.time_ns(),
            status_code=200,
            attrs={"db.system": "dynamodb", "db.operation": "PutItem", "peer.service": "dynamodb"},
        )

        log.info(f"Evento {event_id} (Flag: {body['flag_name']}) salvo no DynamoDB.")

        # Se tudo deu certo, deleta a mensagem da fila
        sqs_client.delete_message(
            QueueUrl=SQS_QUEUE_URL,
            ReceiptHandle=message['ReceiptHandle']
        )

        emit_otlp_span(
            trace_id=trace_id,
            span_id=consumer_span_id,
            parent_span_id=parent_span_id,
            name="SQS Process EvaluationEvent",
            kind=2,
            start_ns=start_ns,
            end_ns=time.time_ns(),
            status_code=200,
            attrs={"messaging.system": "aws_sqs", "flag.name": body['flag_name']},
        )
        emit_otlp_log(
            "INFO",
            f"Evento SQS processado e salvo no DynamoDB (flag={body['flag_name']}, event_id={event_id})",
            200,
            "sqs/process_message",
            trace_id,
            consumer_span_id,
        )

    except json.JSONDecodeError:
        log.error(f"Erro ao decodificar JSON da mensagem ID: {message['MessageId']}")
        emit_otlp_log("ERROR", f"Erro JSONDecodeError na mensagem SQS {message['MessageId']}", 500, "sqs/process_message", trace_id, consumer_span_id)
    except ClientError as e:
        log.error(f"Erro do Boto3 (DynamoDB ou SQS) ao processar {message['MessageId']}: {e}")
        emit_otlp_log("ERROR", f"Erro Boto3 ao processar mensagem SQS: {e}", 500, "sqs/process_message", trace_id, consumer_span_id)
    except Exception as e:
        log.error(f"Erro inesperado ao processar {message['MessageId']}: {e}")
        emit_otlp_log("ERROR", f"Erro inesperado no worker SQS: {e}", 500, "sqs/process_message", trace_id, consumer_span_id)

def sqs_worker_loop():
    """ Loop principal do worker que ouve a fila SQS """
    log.info("Iniciando o worker SQS...")
    while True:
        try:
            # Long-polling: espera até 20s por mensagens (incluindo MessageAttributes para W3C TraceContext)
            response = sqs_client.receive_message(
                QueueUrl=SQS_QUEUE_URL,
                MaxNumberOfMessages=10,
                WaitTimeSeconds=20,
                MessageAttributeNames=['All']
            )
            
            messages = response.get('Messages', [])
            if not messages:
                # Nenhuma mensagem, continua o loop
                continue
                
            log.info(f"Recebidas {len(messages)} mensagens.")
            
            for message in messages:
                process_message(message)
                
        except ClientError as e:
            log.error(f"Erro do Boto3 no loop principal do SQS: {e}")
            time.sleep(10) # Pausa antes de tentar novamente
        except Exception as e:
            log.error(f"Erro inesperado no loop principal do SQS: {e}")
            time.sleep(10)

# --- Servidor Flask (Health Check + Telemetria/Chaos) ---

app = Flask(__name__)
instrument_flask_app(app)

@app.route('/health')
@app.route('/analytics-api/health')
@app.route('/')
def health():
    # Uma verificação de saúde real poderia checar a conexão com o DynamoDB/SQS
    return jsonify({"status": "ok"})

# --- Inicialização ---

def start_worker():
    """ Inicia o worker SQS em uma thread separada """
    worker_thread = threading.Thread(target=sqs_worker_loop, daemon=True)
    worker_thread.start()

# Inicia o worker SQS em uma thread de background
# Isso garante que ele inicie tanto com 'flask run' quanto com 'gunicorn'
start_worker()

if __name__ == '__main__':
    port = int(os.getenv("PORT", 8005))
    app.run(host='0.0.0.0', port=port, debug=False)