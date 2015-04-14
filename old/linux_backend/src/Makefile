default: all

# Proxy any target to the Makefiles in the per-tool directories
%:
	cd oom && $(MAKE) $@
	cd nstar && $(MAKE) $@
	cd repquota && $(MAKE) $@

.PHONY: default
