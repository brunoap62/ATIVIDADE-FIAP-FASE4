import os
import sys
from pathlib import Path
from unittest.mock import patch, MagicMock
import pytest

# Adiciona o diretório raiz do flag-service ao sys.path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

os.environ["DATABASE_URL"] = "postgresql://localhost:5432/flag_db_test"
os.environ["AUTH_SERVICE_URL"] = "http://auth-service"


@pytest.fixture(autouse=True)
def mock_db_pool():
    with patch("psycopg2.pool.SimpleConnectionPool") as mock_pool_cls:
        mock_pool = MagicMock()
        mock_conn = MagicMock()
        mock_cursor = MagicMock()
        mock_pool.getconn.return_value = mock_conn
        mock_conn.cursor.return_value.__enter__.return_value = mock_cursor
        mock_pool_cls.return_value = mock_pool
        yield mock_pool


def test_health_check(mock_db_pool):
    from app import app
    client = app.test_client()
    response = client.get("/health")
    assert response.status_code == 200
    data = response.get_json()
    assert "status" in data
    assert "ok" in data["status"]
