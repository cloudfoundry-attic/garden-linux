default: all

# Proxy any target to the Makefiles in the per-tool directories
%:
ifndef USE_GSH_GSHD
	cd wsh && $(MAKE) $@
endif
	cd oom && $(MAKE) $@
	cd nstar && $(MAKE) $@
	cd repquota && $(MAKE) $@

.PHONY: default
