## Tekton integration

The CLI does not depend on Tekton, but made with it in mind, so the CLI can be easily used in Tekton tasks.
A simplified example:
```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: apply-tags
spec:
  description: Applies additional tags to the built image.
  params:
  - name: IMAGE_URL
    description: Image repository and tag reference of the the built image.
    type: string
  - name: IMAGE_DIGEST
    description: Image digest of the built image.
    type: string
  - name: ADDITIONAL_TAGS
    description: Additional tags that will be applied to the image in the registry.
    type: array
    default: []
  steps:
    - name: apply-additional-tags
      image: quai.io/org/tekton-catalog/konflux-build-cli:latest
      command: ["konflux-build-cli", "image", "apply-tags"]
      args:
        - --image-url
        - $(params.IMAGE_URL)
        - --digest
        - $(params.IMAGE_DIGEST)
        - --tags
        - $(params.ADDITIONAL_TAGS[*])
```