import os
import sys
from pathlib import Path
from unittest.mock import patch, MagicMock
import pytest

# Adiciona o diretório raiz do analytics-service ao sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

os.environ["AWS_REGION"] = "us-east-2"
os.environ["AWS_SQS_URL"] = "https://sqs.us-east-2.amazonaws.com/123/queue"
os.environ["AWS_DYNAMODB_TABLE"] = "ToggleMasterAnalytics"


@pytest.fixture(autouse=True)
def mock_boto3():
    with patch("boto3.Session") as mock_session:
        mock_client = MagicMock()
        mock_session.return_value.client.return_value = mock_client
        with patch("threading.Thread"):
            yield mock_client


def test_health_check(mock_boto3):
    from app import app
    client = app.test_client()
    response = client.get("/health")
    assert response.status_code == 200
    data = response.get_json()
    assert "status" in data
    assert "ok" in data["status"]


def test_analytics_health_check(mock_boto3):
    from app import app
    client = app.test_client()
    response = client.get("/analytics-api/health")
    assert response.status_code == 200
    data = response.get_json()
    assert "status" in data
