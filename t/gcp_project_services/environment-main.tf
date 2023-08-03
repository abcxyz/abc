locals {
  project_id = "{{.project_id}}"
}

resource "google_project_service" "services" {
  for_each = toset([
    {{/* Replace comma with quote-comma-newline-fourspaces. Because TODO */ -}}
    "{{replaceAll .services "," "\",\n    \""}}",
  ])

  project = local.project_id

  service                    = each.value
  disable_on_destroy         = false
  disable_dependent_services = false
}
