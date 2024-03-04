generate: validate-env-vars
	chmod +x ./scripts/generate.sh && ./scripts/generate.sh

validate-env-vars:
ifndef PROTO_VERSION
	$(error PROTO_VERSION is undefined)
endif
