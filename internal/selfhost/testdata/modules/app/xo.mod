module example.com/app
edition 2026

require example.com/tick v1.0.0 uses clock
require example.com/lib v1.0.0 uses none
require example.com/Acme/util v1.0.0

deny example.com/app/core -> example.com/app/web/...
deny example.com/app/ok -> example.com/app/core
