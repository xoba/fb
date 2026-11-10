// Print labels for an in-memory file catalog.
#import <Foundation/Foundation.h>

@interface Entry : NSObject
@property (nonatomic, copy) NSString *name;
@property (nonatomic) NSUInteger bytes;
- (NSString *)label;
@end

@implementation Entry
- (NSString *)label {
    return [NSString stringWithFormat:@"%@: %lu bytes", self.name, (unsigned long)self.bytes];
}
@end

int main(int argc, const char *argv[]) {
    @autoreleasepool {
        NSDictionary<NSString *, NSNumber *> *sizes = @{@"notes.md": @120, @"letter.eml": @240};
        for (NSString *name in [sizes.allKeys sortedArrayUsingSelector:@selector(compare:)]) {
            Entry *entry = [[Entry alloc] init];
            entry.name = name;
            entry.bytes = sizes[name].unsignedIntegerValue;
            NSLog(@"%@", [entry label]);
        }
    }
    return 0;
}
