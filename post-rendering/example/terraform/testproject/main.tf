provider "google" {
  project = "your-gcp-project-id"
  region  = "us-central1"
}

resource "google_compute_instance" "my_vm" {
  name         = "test-vm"
  zone         = "us-central1-a"
  machine_type = "e2-medium"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-11"
    }
  }
  network_interface {
    network = "default"
  }
}