-- One atomic operation covers every matching rule. Redis TIME avoids node clock skew.
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local operation = ARGV[1]
local id = ARGV[2]
local cost = tonumber(ARGV[3])
local lease_ms = 120000
local states = {}
local function load(i)
 local key = KEYS[2*i-1]
 local active = KEYS[2*i]
 local expired = redis.call('ZRANGEBYSCORE', active, '-inf', now)
 local reserved = tonumber(redis.call('HGET', key, 'reserved') or '0')
 for _,member in ipairs(expired) do
  reserved = reserved - tonumber(redis.call('HGET', key, 'a:'..member) or '0')
  redis.call('HDEL', key, 'a:'..member)
 end
 if #expired > 0 then redis.call('ZREMRANGEBYSCORE', active, '-inf', now) end
 local rpm = tonumber(ARGV[3*i+1])
 local tpm = tonumber(ARGV[3*i+2])
 local concurrent = tonumber(ARGV[3*i+3])
 local updated = tonumber(redis.call('HGET', key, 'updated') or now)
 local elapsed = math.max(0, now-updated)
 local r = math.max(0, tonumber(redis.call('HGET', key, 'r') or '0') - elapsed*rpm/60000)
 local t = math.max(0, tonumber(redis.call('HGET', key, 't') or '0') - elapsed*tpm/60000)
 local count = redis.call('ZCARD', active)
 return {key=key,active=active,reserved=math.max(0,reserved),rpm=rpm,tpm=tpm,concurrent=concurrent,r=r,t=t,count=count}
end
local function save(s)
 redis.call('HSET',s.key,'r',s.r,'t',s.t,'reserved',s.reserved,'updated',now)
 local debt = 0
 if s.rpm>0 then debt=math.max(debt,s.r/s.rpm*60000) end
 if s.tpm>0 then debt=math.max(debt,s.t/s.tpm*60000) end
 redis.call('PEXPIRE',s.key,math.ceil(math.min(2147483647000,math.max(180000,debt+180000))))
 redis.call('PEXPIRE',s.active,180000)
end
if operation=='renew' then
 -- Arguments are lease ID + rule count, repeated; one network call per node/tick.
 local index=2
 local key_index=1
 while index<=#ARGV do
  local member=ARGV[index]
  local size=tonumber(ARGV[index+1])
  for j=1,size do
   local key=KEYS[key_index]
   local active=KEYS[key_index+1]
   if redis.call('ZSCORE',active,member) then
    redis.call('ZADD',active,'XX',now+lease_ms,member)
    local ttl=redis.call('PTTL',key)
    if ttl<180000 then redis.call('PEXPIRE',key,180000) end
    redis.call('PEXPIRE',active,180000)
   end
   key_index=key_index+2
  end
  index=index+2
 end
 return {1}
end
for i=1,#KEYS/2 do states[i]=load(i) end
if operation=='take' then
 for i,s in ipairs(states) do
  local dimension=0
  local wait=1
  if s.rpm>0 and s.r+1>s.rpm+0.000000001 then
   dimension=1;wait=math.max(1,math.ceil((s.r+1-s.rpm)/s.rpm*60))
  elseif s.tpm>0 and s.t+s.reserved+cost>s.tpm+0.000000001 then
   dimension=2;wait=math.max(1,math.ceil(math.max(0,s.t+s.reserved+cost-s.tpm)/s.tpm*60))
  elseif s.concurrent>0 and s.count>=s.concurrent then dimension=3 end
  if dimension>0 then
   -- Persist cleanup/refill, but do not consume ANY rule on a rejected acquisition.
   for _,state in ipairs(states) do save(state) end
   return {0,i,dimension,math.min(wait,86400)}
  end
 end
 for _,s in ipairs(states) do
  if s.rpm>0 then s.r=s.r+1 else s.r=0 end
  if s.tpm>0 then s.reserved=s.reserved+cost;redis.call('HSET',s.key,'a:'..id,cost) end
  redis.call('ZADD',s.active,now+lease_ms,id)
  save(s)
 end
 return {1}
elseif operation=='finish' then
 for _,s in ipairs(states) do
  if redis.call('ZREM',s.active,id)>0 then
   local estimate=tonumber(redis.call('HGET',s.key,'a:'..id) or '0')
   s.reserved=math.max(0,s.reserved-estimate)
   redis.call('HDEL',s.key,'a:'..id)
   if s.tpm>0 then s.t=s.t+math.max(0,cost) end
  end
  save(s)
 end
 return {1}
elseif operation=='inspect' then
 local result={}
 for _,s in ipairs(states) do
  save(s)
  table.insert(result,math.ceil(s.r));table.insert(result,math.ceil(s.t+s.reserved));table.insert(result,s.count)
 end
 return result
end
return redis.error_reply('invalid operation')
