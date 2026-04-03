module github.com/club-1/homard

go 1.18

// Remove when https://github.com/emersion/go-milter/pull/35 is merged
replace github.com/emersion/go-milter v0.4.1 => github.com/n-peugnet/go-milter v0.4.2-0.20260403195049-88504148bc82

require (
	github.com/BurntSushi/toml v1.5.0
	github.com/emersion/go-milter v0.4.1
	github.com/emersion/go-msgauth v0.7.0
)

require github.com/emersion/go-message v0.18.1 // indirect
