-- A Wireshark protocol dissector for our example program.
--
-- For simplicity, we have define one dissector that handles both our metadata
-- fields (the type byte) as well as the data fields inside the plugin_request
-- and plugin_response structs.
--
-- If we had a lot of metadata, we might want to split it into two dissectors,
-- one to handle the metadata bytes and another to handle the actual data.
--
-- Wireshark will use this dissector to parse and display the content of the
-- pcap file emitted by modified-example-program.cc.
--
-- To use the dissector, launch wireshark from the command line:
--
--     wireshark -X lua_script:example-protocol-dissector.lua packet_dump.pcap

local example_proto = Proto(
   'example_proto',
   'Example Protocol'
)

local type_field = ProtoField.uint8(
   'example_proto.type',
   'Request or Response',
   base.DEC,
   {
      [0] = 'plugin request',
      [1] = 'plugin response',
   },
   nil,
   'Does this packet represent a plugin request or a plugin response?'
)

local request_tag_field = ProtoField.uint32(
   'example_proto.request_tag',
   'plugin_request.tag',
   base.DEC,
   nil,
   nil,
   'tag field of the plugin_request struct'
)

local request_x_field = ProtoField.uint32(
   'example_proto.request_x',
   'plugin_request.x',
   base.DEC,
   nil,
   nil,
   'x field of the plugin_request struct'
)

local request_y_field = ProtoField.uint32(
   'example_proto.request_y',
   'plugin_request.y',
   base.DEC,
   nil,
   nil,
   'y field of the plugin_request struct'
)

local response_tag_field = ProtoField.uint32(
   'example_proto.response_tag',
   'plugin_response.tag',
   base.DEC,
   nil,
   nil,
   'tag field of the plugin_response struct'
)

local response_z_field = ProtoField.uint32(
   'example_proto.response_z',
   'plugin_response.z',
   base.DEC,
   nil,
   nil,
   'z field of the plugin_response struct'
)

example_proto.fields = {
   type_field,
   request_tag_field,
   request_x_field,
   request_y_field,
   response_tag_field,
   response_z_field,
}

-- Our dissection function.  It is called once on each packet.  It can parse
-- bits of the buffer, and mark them as belonging to different fields.
function example_proto.dissector(buf, pkt, tree)
   -- Mark the whole buffer as being parsed by our example_proto.  buf(0)
   -- creates a new view of the buffer from byte 0 to the end.
   local subtree = tree:add(example_proto, buf(0))

   local pos = 0

   local type_byte = buf(pos, 1):le_uint()
   subtree:add(type_field, buf(pos,1), type_byte)
   pos = pos + 1

   if type_byte == 0 then
      subtree:add(request_tag_field, buf(pos,4), buf(pos,4):le_uint())
      pos = pos + 4
      subtree:add(request_x_field, buf(pos,4), buf(pos,4):le_uint())
      pos = pos + 4
      subtree:add(request_y_field, buf(pos,4), buf(pos,4):le_uint())
      pos = pos + 4
   else
      subtree:add(response_tag_field, buf(pos,4), buf(pos,4):le_uint())
      pos = pos + 4
      subtree:add(response_z_field, buf(pos,4), buf(pos,4):le_uint())
      pos = pos + 4
   end

   -- If we wanted to hand off part of the packet to another dissector, we would
   -- do this:
   --
   -- local sub_dis = Dissector.get('sub-protocol-name')
   -- sub_dis.call(buf(pos):tvb(), pkt, tree)
end

-------------------------------------------------------------------------------
-- Register the outer dissector as the USER0 handler.
--------------------------------------------------------------------------------
local wtap_encap_table = DissectorTable.get('wtap_encap')
wtap_encap_table:add(wtap.USER0, example_proto)
