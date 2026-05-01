BACKEND_DATA_DIR ?= /tmp/forgify-dev
LOG_FILE         := /tmp/forgify-dev.log
PORT             ?= 8742

testend:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@sleep 0.3
	@cd backend && \
	  go run ./cmd/server \
	    --dev \
	    --port $(PORT) \
	    --data-dir $(BACKEND_DATA_DIR) \
	    --collections-dir $(shell pwd)/testend/collections \
	    --integration-dir $(shell pwd)/testend \
	  > $(LOG_FILE) 2>&1 &
	@echo "→ Waiting for backend..."
	@while ! curl -sf http://localhost:$(PORT)/api/v1/health > /dev/null 2>&1; do sleep 0.5; done
	@echo "→ http://localhost:$(PORT)/dev/  (logs → $(LOG_FILE))"
	@open http://localhost:$(PORT)/dev/

stop:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@echo "→ Stopped"

logs:
	@tail -f $(LOG_FILE)

clear:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@rm -rf $(BACKEND_DATA_DIR)
	@rm -f  $(LOG_FILE)
	@echo "→ Cleared (db + attachments + log)"

.PHONY: setup testend stop logs clear
