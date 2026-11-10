// Storage for the file catalog, with one blob container per kind.
param location string = resourceGroup().location
param kinds array = [
  'notes'
  'letters'
]

resource store 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'catalog${uniqueString(resourceGroup().id)}'
  location: location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
}

resource blobs 'Microsoft.Storage/storageAccounts/blobServices@2023-01-01' = {
  parent: store
  name: 'default'
}

resource containers 'Microsoft.Storage/storageAccounts/blobServices/containers@2023-01-01' = [for kind in kinds: {
  parent: blobs
  name: kind
}]

output accountName string = store.name
