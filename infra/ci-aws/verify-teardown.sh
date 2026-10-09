#!/usr/bin/env bash
# Prints what is left in the account tagged project=xo-ci (instances,
# key pair, security group). Every list must be empty after a destroy.
set -uo pipefail
echo "instances (pending/running/stopping/stopped):"
aws ec2 describe-instances --region us-east-1 \
	--filters Name=tag:project,Values=xo-ci Name=instance-state-name,Values=pending,running,stopping,stopped \
	--query 'Reservations[].Instances[].InstanceId' --output json
echo "key pairs named xo-ci-key:"
aws ec2 describe-key-pairs --region us-east-1 --filters Name=key-name,Values=xo-ci-key \
	--query 'KeyPairs[].KeyName' --output json
echo "security groups named xo-ci-ssh:"
aws ec2 describe-security-groups --region us-east-1 --filters Name=group-name,Values=xo-ci-ssh \
	--query 'SecurityGroups[].GroupId' --output json
echo "volumes tagged project=xo-ci:"
aws ec2 describe-volumes --region us-east-1 --filters Name=tag:project,Values=xo-ci \
	--query 'Volumes[].VolumeId' --output json
