IMAGE   := provisioner:latest
CLUSTER := take-home
PORT    := 8080

.PHONY: up down cluster provisioner provisioner-local reset logs check crd-install controller-run \
	sample-create sample-check sample-delete debug

up: cluster provisioner
cluster:
	@kind get clusters 2>/dev/null | grep -qx $(CLUSTER) \
		|| kind create cluster --name $(CLUSTER)
	@kubectl config use-context kind-$(CLUSTER)

provisioner:
	@docker build -q -t $(IMAGE) ./provisioner
	@docker rm -f provisioner >/dev/null 2>&1 || true
	@docker run -d --name provisioner -p $(PORT):8080 $(IMAGE) >/dev/null
	@echo "provisioner is listening on http://localhost:$(PORT)"

provisioner-local:
	@echo "provisioner will listen on http://localhost:$(PORT), stop it with ctrl-c"
	@cd provisioner && go run . -addr=:$(PORT)

reset:
	@docker rm -f provisioner >/dev/null 2>&1 || true
	@$(MAKE) --no-print-directory provisioner

logs:
	@docker logs -f provisioner

check:
	@curl -sf http://localhost:$(PORT)/healthz >/dev/null && echo "provisioner: ok" || echo "provisioner: down"
	@kubectl cluster-info --context kind-$(CLUSTER) >/dev/null 2>&1 && echo "cluster: ok" || echo "cluster: down"

down:
	@docker rm -f provisioner >/dev/null 2>&1 || true
	@kind delete cluster --name $(CLUSTER)

crd-install:
	@kubectl apply -f controller/config/crd.yaml

controller-run: crd-install
	@cd controller && go run ./cmd -provisioner-url=http://localhost:$(PORT)

sample-create:
	@kubectl apply -f controller/config/sample.yaml

sample-check:
	@kubectl get -f controller/config/sample.yaml -o wide

sample-delete:
	@kubectl delete -f controller/config/sample.yaml

debug:
	@curl -s http://localhost:$(PORT)/_debug/databases
