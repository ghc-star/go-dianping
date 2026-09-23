-- KEYS[1] 库存 String
-- KEYS[2] 活动信息 Hash
-- KEYS[3] 已购用户 Hash
-- KEYS[4] 订单 Stream
-- KEYS[5] 订单受理状态 String
--
-- ARGV[1] 当前时间，Unix 毫秒
-- ARGV[2] 用户 ID
-- ARGV[3] 优惠券 ID
-- ARGV[4] 订单 ID
--
-- 返回：
--  1  成功
-- -1  活动不存在或数据不完整
-- -2  尚未开始
-- -3  已经结束
-- -4  库存不足
-- -5  重复购买

if redis.call('EXISTS',KEYS[2])==0 then
  return -1
end

local meta=redis.call('HMGET',KEYS[2],'begin','end')
local beginTime=tonumber(meta[1])
local endTime=tonumber(meta[2])
local now=tonumber(ARGV[1])
if not beginTime or not endTime or not now then
  return -1
end

if now<beginTime then 
  return -2
end

if now>=endTime then
  return -3
end

if redis.call('HEXISTS',KEYS[3],ARGV[2])==1 then
  return -5
end

local stock=tonumber(redis.call('GET',KEYS[1]))
if not stock or stock <= 0 then
  return -4
end

redis.call('DECR', KEYS[1])

redis.call(
  'XADD', KEYS[4], '*',
  'orderId', ARGV[4],
  'userId', ARGV[2],
  'voucherId', ARGV[3]
)

redis.call('HSET', KEYS[3], ARGV[2], ARGV[4])
redis.call('SET', KEYS[5], ARGV[2], 'EX', 86400)

return 1