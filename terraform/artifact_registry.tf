resource "google_artifact_registry_repository" "online_boutique" {
  location      = var.region
  repository_id = "online-boutique"
  description   = "Docker images for Online Boutique"
  format        = "DOCKER"

  project = var.gcp_project_id

  depends_on = [
    module.enable_google_apis
  ]
}