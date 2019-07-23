// Program Trace Analysis with Wireshark --- Modified Example Program

#include <sys/time.h>

#include <cstdint>
#include <cstdio>
#include <cstdlib>

static FILE *dump_packets_file = nullptr;
void open_dump_packets_file() {
  dump_packets_file = fopen("packet_dump.pcap", "w");
  if (dump_packets_file == NULL) {
    exit(1);
  }

  // Write pcap header.

  // Magic value --- used to detect the endianness of the file, so that we can
  // write in whatever our native endianness is.
  uint32_t magic = 0xa1b2c3d4;
  fwrite(&magic, 1, sizeof(magic), dump_packets_file);

  // Pcap version
  uint16_t version_major = 2;
  fwrite(&version_major, 1, sizeof(version_major), dump_packets_file);
  uint16_t version_minor = 4;
  fwrite(&version_minor, 1, sizeof(version_minor), dump_packets_file);

  // Timezone offset from GMT.  Almost always left at 0.
  int32_t thiszone = 0;
  fwrite(&thiszone, 1, sizeof(thiszone), dump_packets_file);

  // Timestamp accuracy.  Left at 0.
  uint32_t sigfigs = 0;
  fwrite(&sigfigs, 1, sizeof(sigfigs), dump_packets_file);

  // Maximum size of an individual "packet".
  uint32_t snaplen = 65536;
  fwrite(&snaplen, 1, sizeof(snaplen), dump_packets_file);

  // Specifies the type of packet we're dumping --- 147 to 162 are reserved for
  // private use, and are referred to as USER0 through USER15 inside of
  // Wireshark.  Pick USER0.  Wireshark won't know how to handle our "packets"
  // by default, but we will fix that with a custom dissector.
  uint32_t network = 147;
  fwrite(&network, 1, sizeof(network), dump_packets_file);

  if (ferror(dump_packets_file)) {
    exit(1);
  }

  if (fflush(dump_packets_file) == EOF) {
    exit(1);
  }
}

void dump_packet(uint8_t type, const char *data, size_t len) {
  if (!dump_packets_file) {
    open_dump_packets_file();
  }

  // We need to log both the actual packet data, as well as some metadata
  // (whether the packet is a plugin request or a plugin response).
  uint32_t encapsulated_len = uint32_t(len) + 1;

  // Write per-packet pcap header.
  timeval now;
  if (gettimeofday(&now, nullptr)) {
    exit(1);
  }

  uint32_t ts_sec = now.tv_sec;
  uint32_t ts_usec = now.tv_usec;
  fwrite(&ts_sec, 1, sizeof(ts_sec), dump_packets_file);
  fwrite(&ts_usec, 1, sizeof(ts_usec), dump_packets_file);
  fwrite(&encapsulated_len, 1, sizeof(encapsulated_len),
         dump_packets_file);  // incl_len
  fwrite(&encapsulated_len, 1, sizeof(encapsulated_len),
         dump_packets_file);  // orig_len

  // Write the per-packet encapsulation header.

  // Is this a plugin request or plugin response?
  fwrite(&type, 1, sizeof(type), dump_packets_file);

  // Write the raw data of our plugin request or response.
  fwrite(data, len, sizeof(char), dump_packets_file);

  if (ferror(dump_packets_file)) {
    exit(1);
  }

  if (fflush(dump_packets_file) == EOF) {
    exit(1);
  }
}

struct plugin_response {
  uint32_t tag;
  uint32_t z;
};

struct plugin_request {
  uint32_t tag;
  uint32_t x;
  uint32_t y;
};

void do_plugin_request(plugin_request *req, plugin_response *resp) {
  // Suspend your disbelief... Pretend this is a complicated plugin system.
  resp->tag = req->tag;
  resp->z = req->x + req->y;
}

int main(int argc, char **argv) {
  for (int i = 0; i < 1000; ++i) {
    plugin_request req = {uint32_t(i), uint32_t(rand()), uint32_t(rand())};
    plugin_response resp;

    dump_packet(0, reinterpret_cast<char *>(&req), sizeof(req));
    do_plugin_request(&req, &resp);
    dump_packet(1, reinterpret_cast<char *>(&resp), sizeof(resp));
  }

  return 0;
}
