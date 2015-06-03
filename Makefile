default: all

# Proxy any target to the Makefile in the old directory
%:
	cd old && $(MAKE) $@

.PHONY: default
