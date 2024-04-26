## SDK - Creating new syntest plugin
.PHONY : new-go-syntest
new-go-syntest:
	@echo Creating Directory plugins/syntests/$(name)
	mkdir agent/plugins/syntests/$(name)

	@echo Creating file plugins/syntests/$(name)/$(name).go
	cp agent/plugins/templates/go/test.tpl.go plugins/syntests/$(name)/$(name).go

	sed -i .backup s/{{TestName}}/$(call capitalize,$(name))/g  plugins/syntests/$(name)/$(name).go
	rm agent/plugins/syntests/$(name)/$(name).go.backup

define capitalize
$(shell echo $(1) | awk '{$$1=toupper(substr($$1,0,1))substr($$1,2); print $$0}')
endef
