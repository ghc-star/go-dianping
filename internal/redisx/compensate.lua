-- KEYS[1] 库存 String
-- KEYS[2] 已购用户 Hash
-- KEYS[3] 订单状态 String
-- KEYS[4] 补偿标记 String
--
-- ARGV[1] 用户 ID
-- ARGV[2] 订单 ID
--
-- 返回：
--  1  本次完成补偿
--  0  已经补偿过

	if redis.call('EXISTS', KEYS[4]) == 1 then
	  return 0
	end

	local stock = redis.call('GET', KEYS[1])
	if stock and not tonumber(stock) then
	  return -1
	end

	if redis.call('SET', KEYS[4], '1', 'NX') == false then
	  return 0
	end

redis.call('INCR', KEYS[1])

if redis.call('HGET', KEYS[2], ARGV[1]) == ARGV[2] then
  redis.call('HDEL', KEYS[2], ARGV[1])
end

redis.call('SET', KEYS[3], 'failed:' .. ARGV[1], 'EX', 86400)

return 1