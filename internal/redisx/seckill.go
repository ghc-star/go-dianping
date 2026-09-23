package redisx

import (
	_ "embed"

	"github.com/redis/go-redis/v9"
)

//go:embed init_seckill.lua
var initSeckillLua string

//go:embed seckill.lua
var seckillLua string

//go:embed compensate.lua
var compensateLua string

var InitSeckillScript = redis.NewScript(initSeckillLua)
var SeckillScript = redis.NewScript(seckillLua)
var CompensateScript = redis.NewScript(compensateLua)
