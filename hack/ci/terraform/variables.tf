terraform {
  backend "gcs" {}
}

provider "google" {
  region  = "europe-west1"
}

variable "cluster_name" {
  default = "capact-stage"
  type    = string
}

variable "location" {
  default = "europe-west1"
  type    = string
}

variable "region" {
  default = "europe-west1"
  type    = string
}

variable "node_pool_name" {
  default = "stage-node-pool"
  type    = string
}

variable "google_compute_network_name" {
  default = "stage-vpc-network"
  type    = string
}

variable "google_compute_subnetwork_name" {
  default = "stage-subnetwork"
  type    = string
}

variable "google_compute_subnetwork_ip_cidr_range" {
  default = "172.16.0.0/28"
  type    = string
}

variable "google_compute_subnetwork_secondary_ip_range_name1" {
  default = "gke-stage-pods"
  type    = string
}

variable "google_compute_subnetwork_secondary_ip_range_cidr1" {
  default = "10.0.0.0/14"
  type    = string
}

variable "google_compute_subnetwork_secondary_ip_range_name2" {
  default = "gke-stage-services"
  type    = string
}

variable "google_compute_subnetwork_secondary_ip_range_cidr2" {
  default = "10.4.0.0/20"
  type    = string
}

variable "google_container_cluster_private_cluster_config_master_ipv4_cidr_block" {
  default = "172.16.10.0/28"
  type    = string
}



variable "project" {
  default = "capact"
  type    = string
}



variable "machine_type" {
  default = "n1-standard-2"
  type    = string
}



variable "preemptible" {
  default = "true"
  type    = string
}

variable "autoscaling_max_node_count" {
  default = 2
  type    = number
}

variable "disk_type" {
  default = "pd-standard"
  type    = string
}

variable "image_type" {
  default = "COS"
  type    = string
}
