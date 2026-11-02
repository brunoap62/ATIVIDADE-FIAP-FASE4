# ==============================================================================
# AWS DynamoDB Table (ToggleMasterAnalytics)
# ==============================================================================
resource "aws_dynamodb_table" "analytics" {
  name         = var.table_name
  billing_mode = var.billing_mode # FinOps: sob demanda / custo zero se ocioso
  hash_key     = var.hash_key

  attribute {
    name = var.hash_key
    type = "S"
  }

  server_side_encryption {
    enabled = true
  }

  tags = {
    Name = var.table_name
  }
}
