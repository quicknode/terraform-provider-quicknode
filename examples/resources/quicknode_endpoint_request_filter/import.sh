# A filter has no natural name, so it is imported by id, as
# "<endpoint id>/<filter id>". Read the ids from the endpoint's security
# settings in the Quicknode dashboard.
terraform import quicknode_endpoint_request_filter.read_only 123456/f1e2d3c4-5b6a-7890-abcd-ef1234567890
