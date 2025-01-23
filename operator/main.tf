## basic terraform needed to standup a node pool in aws account
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 2.7.0"
    }
  }
  backend "http" {
    lock_method    = "POST"
    unlock_method  = "DELETE"
    retry_wait_min = 5
    address        = "https://gitlab-master.nvidia.com/api/v4/projects/114559/terraform/state/aws-node-pools"
  }
}

provider "aws" {
  region = "us-west-2"
  default_tags {
    tags = {
      created_by = "terraform"
      managed_by = "terraform"
      owned_by   = "baseos infra"
    }
  }
}

variable "cluster_name" {
  type = string
}

data "terraform_remote_state" "aws_eks_cluster" {
  backend = "http"
  config = {
    address = "https://gitlab-master.nvidia.com/api/v4/projects/4478/terraform/state/aws-cluster-${var.cluster_name}" ## note this has not yet been migrated to skysmyith yet
  }
}

data "terraform_remote_state" "aws_common" {
  backend = "http"
  config = {
    address = "https://gitlab-master.nvidia.com/api/v4/projects/66987/terraform/state/aws"
  }
}

// https://cloud-images.ubuntu.com/aws-eks/
// https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/ami
data "aws_ami" "eks_ubunut_latest" {
  most_recent = true
  owners      = ["099720109477"] // Canonical

  // list of filter keys https://docs.aws.amazon.com/cli/latest/reference/ec2/describe-images.html
  filter {
    name   = "name"
    values = ["ubuntu-eks/k8s_${local.k8s_version}/images/hvm-ssd/ubuntu-${local.ubuntu_versions_map[local.ubuntu_version]}-${local.ubuntu_version}-*"]
  }
  filter {
    name   = "architecture"
    values = ["x86_64"]
  }
}

data "aws_iam_role" "cluster_role" {
  name = "${var.cluster_name}-node-role"
}

data "aws_region" "current" {}

locals {
  k8s_version      = "1.30"
  ubuntu_version   = "22.04"
  cluster_dns_ip   = "172.20.0.10"
  cluster_ca       = data.terraform_remote_state.aws_eks_cluster.outputs.cluster.certificate_authority[0].data
  cluster_endpoint = data.terraform_remote_state.aws_eks_cluster.outputs.cluster.endpoint
  cluster_security_groups = setunion(
    data.terraform_remote_state.aws_eks_cluster.outputs.cluster.vpc_config[0].security_group_ids,
    [data.terraform_remote_state.aws_eks_cluster.outputs.cluster.vpc_config[0].cluster_security_group_id]
  )

  ubuntu_versions_map = {
    "22.04" = "jammy"
    "24.04" = "nobel"
  }

  region        = data.aws_region.current.name
  subnet_ids    = [for subnet in data.terraform_remote_state.aws_common.outputs.simple-vpcs.vms[local.region].public_subnets : subnet.id]
  node_role_arn = data.aws_iam_role.cluster_role.arn

  eks_ubunut_latest_image = {
    image            = data.aws_ami.eks_ubunut_latest.id
    root_device_name = data.aws_ami.eks_ubunut_latest.root_device_name
  }

  node_pools = [
    {
      name             = "skyhooke2e"
      instance_types   = ["t3.medium"]
      node_count       = 1
    },
  ]
}

resource "aws_eks_node_group" "this" {
  for_each = { for np in local.node_pools : np.name => np }

  cluster_name    = var.cluster_name
  node_group_name = each.key
  node_role_arn   = local.node_role_arn
  subnet_ids      = local.subnet_ids

  labels = {
    "skyhook.nvidia.com/test-node" = each.value.name
  }

  scaling_config {
    desired_size = each.value.node_count
    max_size     = each.value.node_count
    min_size     = each.value.node_count
  }

  instance_types = each.value.instance_types

  ami_type = "CUSTOM"

  launch_template {
    id      = aws_launch_template.this[each.key].id
    version = aws_launch_template.this[each.key].latest_version
  }

  tags = {
    Name       = each.key
  }
}

resource "aws_launch_template" "this" {
  for_each = { for np in local.node_pools : np.name => np }
  # only create a launch template for AMI images
  name     = each.key
  image_id = local.eks_ubunut_latest_image.image

  user_data = base64encode(<<EOF
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="//"

--//
Content-Type: text/x-shellscript; charset="us-ascii"

#!/bin/bash

set -ex

/etc/eks/bootstrap.sh ${var.cluster_name} \
  --kubelet-exta-args \
  '--node-labels=eks.amazonaws.com/nodegroup-image=${local.eks_ubunut_latest_image.image},eks.amazonaws.com/capacityType=ON_DEMAND,eks.amazonaws.com/nodegroup=${each.value.name} \
  --max-pods=17' \
  --apiserver-endpoint ${local.cluster_endpoint} \
  --b64-cluster-ca ${local.cluster_ca} \
  --dns-cluster-ip ${local.cluster_dns_ip} \
  --container-runtime containerd \
  --use-max-pods false

--//--
EOF
  )

  block_device_mappings {
    device_name = local.eks_ubunut_latest_image.root_device_name
    ebs {
      volume_size           = 20
      volume_type           = "gp2"
      delete_on_termination = true
    }
  }
  vpc_security_group_ids = local.cluster_security_groups

  monitoring {
    enabled = true
  }
  private_dns_name_options {
    enable_resource_name_dns_a_record = true
  }

  dynamic "tag_specifications" {
    for_each = {
      for type in local.asg_resources_to_tag : type => data.aws_default_tags.default_tags
    }
    content {
      resource_type = tag_specifications.key
      tags = merge(
        tag_specifications.value.tags,
        {
          "Name" = "eks-${var.cluster_name}-${each.key}"
        }
      )
    }
  }
}

locals {
  asg_resources_to_tag = ["instance", "volume", "network-interface"] # you may have more here
}

data "aws_default_tags" "default_tags" {}