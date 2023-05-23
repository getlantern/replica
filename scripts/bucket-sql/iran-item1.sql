 -- TODO: Link to item in grant that this solves.
with external_uploads as (
    select 
        strftime('%Y-%m', datetime(metadata->'retrieval_timestamp', 'unixepoch')) as month,
        metadata->>'external_source' as source
    from metadata
)
select
    count(*),
    coalesce(source->>'description', source->>'alternative_channel_name', source->>'url') as human
from external_uploads
where source is not null and month between '2023-01' and '2023-03'
group by human;