terraform {
  backend "gcs" {
    bucket = "{{.bucket_name}}"
    prefix = "{{.bucket_prefix}}/resources/{{.product_id}}/{{.environment}}"
  }
}
