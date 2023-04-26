# You should invoke this passing BUCKET. See the README.

# stop attempts to make .json.o (wtf?)
.SUFFIXES:

BUCKET_JSON_FILE = $(BUCKET).json

define INSERT_SQL
insert into object \
	select value->>'Key', value->>'LastModified', value->>'ETag', value->>'Size', '$(BUCKET)' \
	from json_each(readfile('$$file'));
endef

SQLITE_CMD := sqlite3 -bail -cmd '.changes on' replica.db

init-schema:
	sqlite3 replica.db < schema

# This deletes any existing files for BUCKET, then adds them using the chunked JSON files. Run this for each bucket you want included in the sqlite db. Depending on the .0 file causes us to recursively generate the rest of the files first. There are multiple files because sqlite barfs on JSON over 1GB.
import-bucket: $(BUCKET_JSON_FILE).0
	$(SQLITE_CMD) "delete from object where bucket='$(BUCKET)'"
	for file in $(BUCKET_JSON_FILE).*; do echo $$file; $(SQLITE_CMD) <<< "$(INSERT_SQL)"; done

# Unadulterated output from s3api list-objects-v2, with a .Contents array.
%.json:
	aws-endpoint-env --profile '$(PROFILE)' s3api list-objects-v2 --bucket $* > $@~
	mv $@~ $@

# We need BUCKET_JSON_FILE, but we don't actually want to trigger on it.
.PHONY: rm-split-files
rm-split-files: $(BUCKET_JSON_FILE)
	-rm -v $(BUCKET_JSON_FILE).*

# If we're calling this, start over by wiping them first.
split-json: rm-split-files $(BUCKET_JSON_FILE).0

# This is recursive. It probably should pass make flags.
$(BUCKET_JSON_FILE).%: $(BUCKET_JSON_FILE)
	jq ".Contents[$*000000:$$((($*+1)*1000000))]" $< > $@
	[[ $$(jq length $@) == 0 ]] || make $(BUCKET_JSON_FILE).$$(($*+1))

.PHONY: get-metadata
get-metadata:
	# aws-endpoint-env is a wrapper around aws that lets me provide an endpoint url from the environment.  There's probably a much better way to do this, like having AWS as make variable.
	aws-endpoint-env --profile $(PROFILE) s3 sync s3://$(BUCKET) $(BUCKET) --exclude '*' --include '*/metadata'

$(BUCKET):
	make get-metadata

import-metadata: $(BUCKET)
	for info_hash_hex in `ls $(BUCKET)`; do \
`# 		echo $$info_hash_hex || break;` \
		sqlite3 replica.db <<< \
			"replace into metadata (info_hash_hex, metadata) values ('$$info_hash_hex', readfile('$(BUCKET)/$$info_hash_hex/metadata'))" \
		|| break; \
	done
