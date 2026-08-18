import sys
from pymilvus import MilvusClient

client = MilvusClient(uri='http://milvus:19530')

collections = client.list_collections()
print('Collections:', collections, flush=True)

for name in collections:
    try:
        stats = client.get_collection_stats(name)
        print(f'Collection: {name}, rows: {stats.get("row_count", "N/A")}', flush=True)
    except Exception as e:
        print(f'Collection: {name}, error: {e}', flush=True)
