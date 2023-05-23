 -- TODO: Link to item in grant that this solves.
with uploads as (
    select 
        strftime('%Y-%m', datetime(metadata->'creation_timestamp', 'unixepoch')) as month,
        metadata->>'request_country' as country,
        metadata->>'uploader' as uploader,
        *
    from metadata
)
select
    count(*)
from uploads
where country='IR' and uploader='AnonymousEndpoint'
and month between '2023-01' and '2023-03'
;
