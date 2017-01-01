// clang++ tlstest.mm -o tlstest -framework Foundation -framework Security -std=c++11

#include <CoreFoundation/CoreFoundation.h>
#import <Foundation/Foundation.h>
#include <Security/SecureTransport.h>

struct StreamPair {
  NSInputStream* in;
  NSOutputStream* out;
};

const size_t kBufferSize = 1024;

NSString* ReadBuffer(NSInputStream* stream) {
  uint8_t buf[kBufferSize];
  NSInteger bytes_read = [stream read:buf maxLength:sizeof(buf)];
  NSString* line  = [[[NSString alloc] initWithBytes:buf
                                              length:bytes_read
                                            encoding:NSASCIIStringEncoding] autorelease];
  NSLog(@">>> %@", line);
  return line;
}

void WriteLine(NSOutputStream* stream, NSString* line) {
  NSLog(@"<<< %@", line);
  [stream write:(const uint8_t*)[line UTF8String] maxLength:[line length]];
  const uint8_t nl = '\n';
  [stream write:&nl maxLength:1];
}

OSStatus MySSLRead(SSLConnectionRef conn, void* data, size_t* data_len) {
  NSInputStream* stream = ((StreamPair*)conn)->in;
  *data_len = [stream read:(uint8_t*)data maxLength:*data_len];
  return noErr;
}

OSStatus MySSLWrite(SSLConnectionRef conn, const void* data, size_t* data_len) {
  NSOutputStream* stream = ((StreamPair*)conn)->out;
  *data_len = [stream write:(uint8_t*)data maxLength:*data_len];
  return noErr;
}

NSString* ReadBuffer(SSLContextRef tlsctx) {
  uint8_t buf[kBufferSize];
  size_t data_len;
  OSStatus status = SSLRead(tlsctx, buf, sizeof(buf), &data_len);
  if (status != noErr) {
    NSLog(@"SSLRead error: %d", status);
    return nil;
  }

  NSString* line  = [[[NSString alloc] initWithBytes:buf
                                              length:data_len
                                            encoding:NSASCIIStringEncoding] autorelease];
  NSLog(@">+> %@", line);
  return line;
}

void WriteLine(SSLContextRef tlsctx, NSString* line) {
  NSLog(@"<+< %@", line);
  size_t processed;
  OSStatus status = SSLWrite(tlsctx, [line UTF8String], [line length], &processed);
  if (status != noErr) {
    NSLog(@"SSLWrite error: %d", status);
    return;
  }

  const char nl = '\n';
  status = SSLWrite(tlsctx, &nl, 1, &processed);
  if (status != noErr) {
    NSLog(@"SSLWrite error: %d", status);
    return;
  }
}

int main() {
  CFReadStreamRef read_cf;
  CFWriteStreamRef write_cf;
  CFStreamCreatePairWithSocketToHost(NULL, CFSTR("localhost"), 9925, &read_cf, &write_cf);

  NSInputStream* is = (NSInputStream*)read_cf;
  NSOutputStream* os = (NSOutputStream*)write_cf;

  [is open];
  [os open];

  ReadBuffer(is);
  WriteLine(os, @"EHLO tlstest");
  ReadBuffer(is);

  StreamPair pair = { is, os };

  SSLContextRef tlsctx = SSLCreateContext(NULL, kSSLClientSide, kSSLStreamType);
  SSLSetIOFuncs(tlsctx, MySSLRead, MySSLWrite);
  SSLSetConnection(tlsctx, &pair);
  SSLSetSessionOption(tlsctx, kSSLSessionOptionBreakOnServerAuth, true);

  WriteLine(os, @"STARTTLS");
  ReadBuffer(is);

  // Skip trust verification.
  OSStatus handshake = SSLHandshake(tlsctx);
  NSLog(@"SSL Handshake = %d", handshake);
  if (handshake == errSSLServerAuthCompleted) {
    handshake = SSLHandshake(tlsctx);
    NSLog(@"SSL Handshake = %d", handshake);
  }

  ReadBuffer(tlsctx);
  WriteLine(tlsctx, @"EHLO tlstest-tls");

  ReadBuffer(tlsctx);
  ReadBuffer(tlsctx);

  WriteLine(tlsctx, @"MAIL FROM:<test@tlstest>");
  ReadBuffer(tlsctx);

  WriteLine(tlsctx, @"RCPT TO:<test@localhost>");
  ReadBuffer(tlsctx);

  WriteLine(tlsctx, @"DATA");
  sleep(10);
  ReadBuffer(tlsctx);

  WriteLine(tlsctx, @"More data");
  WriteLine(tlsctx, @"and some more");
  WriteLine(tlsctx, @"more more more");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @"adfasdf;lan io;aweofani ef;awe");
  WriteLine(tlsctx, @".");

  ReadBuffer(tlsctx);

  WriteLine(tlsctx, @"QUIT");
  ReadBuffer(tlsctx);

  SSLClose(tlsctx);

  [is close];
  [os close];
}
