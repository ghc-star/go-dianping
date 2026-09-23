-- KEYS[1] 库存 String
-- KEYS[2] 活动信息 Hash
-- KEYS[3] 已购用户 Hash
--
-- ARGV[1] 库存
-- ARGV[2] 开始时间，Unix 毫秒
-- ARGV[3] 结束时间，Unix 毫秒

if redis.call('EXISTS',KEYS[1])==1
  or redis.call('EXISTS',KEYS[2])==1
  or redis.call('EXISTS',KEYS[3])==1 then
  return 1
  end

redis.call('SET',KEYS[1],ARGV[1])

redis.call(
  'HSET',
  KEYS[2],
  'begin',ARGV[2],
  'end',ARGV[3]
)

redis.call(
	'HSET',
	KEYS[3],
	'__initialized', '1'
)

return 0